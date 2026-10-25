package server

import (
	"encoding/json"
	"log"
	"net/http"

	"bootimus/internal/deployment"
	"bootimus/internal/diskimage"
	"bootimus/internal/models"
)

func (s *Server) profileLookup() deployment.ProfileLookup {
	if s.config.Storage == nil {
		return nil
	}
	return s.config.Storage.GetDeploymentProfile
}

func (s *Server) effectiveFor(client *models.Client) (deployment.Effective, *models.ClientGroup) {
	group := s.clientGroupOf(client)
	return deployment.Resolve(client, group, s.profileLookup()), group
}

func (s *Server) applyProfileImages(images []models.Image, client *models.Client, group *models.ClientGroup, eff deployment.Effective) []models.Image {
	if client == nil || s.config.Storage == nil {
		return images
	}
	if !eff.ShowPublicImages && client.ShowPublicImages {
		explicit := map[string]bool{}
		for _, f := range client.AllowedImages {
			explicit[f] = true
		}
		if group != nil {
			for _, f := range group.AllowedImages {
				explicit[f] = true
			}
		}
		if eff.ImageFilename != "" {
			explicit[eff.ImageFilename] = true
		}
		kept := images[:0]
		for _, img := range images {
			if !img.Public || explicit[img.Filename] {
				kept = append(kept, img)
			}
		}
		images = kept
	}
	if eff.ImageFilename == "" {
		return images
	}
	for _, img := range images {
		if img.Filename == eff.ImageFilename {
			return images
		}
	}
	img, err := s.config.Storage.GetImage(eff.ImageFilename)
	if err != nil || img == nil || !img.Enabled {
		return images
	}
	return append(images, *img)
}

func (s *Server) profileInstallImageID(client *models.Client, eff deployment.Effective, images []models.Image) uint {
	if client == nil || s.config.Storage == nil || eff.FirstBootAction != models.FirstBootActionInstall || eff.ImageFilename == "" {
		return 0
	}
	var imageID uint
	for _, img := range images {
		if img.Filename == eff.ImageFilename && img.Enabled {
			imageID = img.ID
			break
		}
	}
	if imageID == 0 {
		return 0
	}
	records, err := s.config.Storage.ListInstallRecords(client.MACAddress, 100)
	if err == nil && deployment.InstallCompletedFor(records, eff.ImageFilename) {
		return 0
	}
	return imageID
}

func (s *Server) ensureFirstBootDeploy(client *models.Client, eff deployment.Effective) {
	if client == nil || s.diskImages == nil || s.config.Storage == nil {
		return
	}
	if eff.FirstBootAction != models.FirstBootActionDeploy || eff.DiskImageID == nil || *eff.DiskImageID == 0 {
		return
	}
	jobs, err := s.config.Storage.ListDiskJobs(client.MACAddress, 200)
	if err != nil {
		log.Printf("Deployment profile: cannot list disk jobs for %s: %v", client.MACAddress, err)
		return
	}
	if deployment.DeployJobExistsFor(jobs, *eff.DiskImageID) {
		return
	}
	job, err := s.diskImages.Deploy(diskimage.DeployRequest{MAC: client.MACAddress, ImageID: *eff.DiskImageID, Disk: diskimage.AutoDisk})
	if err != nil {
		log.Printf("Deployment profile: first-boot deploy for %s not queued: %v", client.MACAddress, err)
		return
	}
	s.logAndBroadcast("Client %s: deployment profile queued first-boot deploy of disk image %d (job %d)", client.MACAddress, *eff.DiskImageID, job.ID)
}

type effectiveView struct {
	deployment.Effective
	MAC              string `json:"mac"`
	ClientName       string `json:"client_name,omitempty"`
	ProfileName      string `json:"profile_name,omitempty"`
	GroupProfileName string `json:"group_profile_name,omitempty"`
	GroupName        string `json:"group_name,omitempty"`
	ImageName        string `json:"image_name,omitempty"`
	SANTargetName    string `json:"san_target_name,omitempty"`
	DiskImageName    string `json:"disk_image_name,omitempty"`
}

func (s *Server) handleClientEffective(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}
	if s.config.Storage == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Deployment profiles require a database"})
		return
	}
	mac := normaliseMenuMAC(r.URL.Query().Get("mac"))
	client, err := s.config.Storage.GetClient(mac)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Client not found"})
		return
	}
	eff, group := s.effectiveFor(client)
	view := effectiveView{Effective: eff, MAC: client.MACAddress, ClientName: client.Name}
	if eff.ClientProfile != nil {
		view.ProfileName = eff.ClientProfile.Name
	}
	if eff.GroupProfile != nil {
		view.GroupProfileName = eff.GroupProfile.Name
	}
	if group != nil {
		view.GroupName = group.Name
	}
	if eff.ImageFilename != "" {
		if img, imgErr := s.config.Storage.GetImage(eff.ImageFilename); imgErr == nil {
			view.ImageName = img.Name
		}
	}
	if eff.SANTargetID != nil {
		if target, tErr := s.config.Storage.GetSANTarget(*eff.SANTargetID); tErr == nil {
			view.SANTargetName = target.Name
		}
	}
	if eff.DiskImageID != nil {
		if di, dErr := s.config.Storage.GetDiskImage(*eff.DiskImageID); dErr == nil {
			view.DiskImageName = di.Name
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": view})
}
