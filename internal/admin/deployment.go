package admin

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"bootimus/internal/models"
	"bootimus/internal/storage"
)

type deploymentProfileView struct {
	*models.DeploymentProfile
	ClientCount int64 `json:"client_count"`
	GroupCount  int64 `json:"group_count"`
}

func (h *Handler) DeploymentProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listDeploymentProfiles(w)
	case http.MethodPost:
		h.createDeploymentProfile(w, r)
	case http.MethodPut:
		h.updateDeploymentProfile(w, r)
	case http.MethodDelete:
		h.deleteDeploymentProfile(w, r)
	default:
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
	}
}

func (h *Handler) listDeploymentProfiles(w http.ResponseWriter) {
	profiles, err := h.storage.ListDeploymentProfiles()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	views := make([]deploymentProfileView, 0, len(profiles))
	for _, p := range profiles {
		clients, groups, countErr := h.storage.CountDeploymentProfileUsers(p.ID)
		if countErr != nil {
			log.Printf("Deployment profiles: failed to count users of profile %d: %v", p.ID, countErr)
		}
		views = append(views, deploymentProfileView{DeploymentProfile: p, ClientCount: clients, GroupCount: groups})
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: views})
}

func (h *Handler) checkDeploymentProfileReferences(p *models.DeploymentProfile) error {
	if p.ImageFilename != "" {
		if _, err := h.storage.GetImage(p.ImageFilename); err != nil {
			return errors.New("image not found: " + p.ImageFilename)
		}
	}
	if p.SANTargetID != nil && *p.SANTargetID != 0 {
		if _, err := h.storage.GetSANTarget(*p.SANTargetID); err != nil {
			return errors.New("SAN target not found")
		}
	}
	if p.DiskImageID != nil && *p.DiskImageID != 0 {
		if _, err := h.storage.GetDiskImage(*p.DiskImageID); err != nil {
			return errors.New("disk image not found")
		}
	}
	return nil
}

func (h *Handler) createDeploymentProfile(w http.ResponseWriter, r *http.Request) {
	var profile models.DeploymentProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}
	profile.ID = 0
	if err := storage.ValidateDeploymentProfile(&profile); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	if err := h.checkDeploymentProfileReferences(&profile); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	if err := h.storage.CreateDeploymentProfile(&profile); err != nil {
		h.sendJSON(w, deploymentProfileErrorStatus(err), Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Deployment profile created - %s", profile.Name)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Deployment profile created", Data: profile})
}

func (h *Handler) updateDeploymentProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := h.deploymentProfileID(w, r)
	if !ok {
		return
	}
	if _, err := h.storage.GetDeploymentProfile(id); err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Deployment profile not found"})
		return
	}
	var profile models.DeploymentProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}
	if err := storage.ValidateDeploymentProfile(&profile); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	if err := h.checkDeploymentProfileReferences(&profile); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	if err := h.storage.UpdateDeploymentProfile(id, &profile); err != nil {
		h.sendJSON(w, deploymentProfileErrorStatus(err), Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Deployment profile updated - %s", profile.Name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Deployment profile updated", Data: profile})
}

func (h *Handler) deleteDeploymentProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := h.deploymentProfileID(w, r)
	if !ok {
		return
	}
	profile, err := h.storage.GetDeploymentProfile(id)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Deployment profile not found"})
		return
	}
	if err := h.storage.DeleteDeploymentProfile(id); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Deployment profile deleted - %s", profile.Name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Deployment profile deleted"})
}

func (h *Handler) deploymentProfileID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil || id == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid deployment profile ID"})
		return 0, false
	}
	return uint(id), true
}

func deploymentProfileErrorStatus(err error) int {
	if errors.Is(err, storage.ErrDeploymentProfileExists) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
