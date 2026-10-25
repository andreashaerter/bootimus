package deployment

import (
	"bootimus/internal/models"
)

const (
	SourceClient       = "client"
	SourceProfile      = "profile"
	SourceGroupProfile = "group-profile"
	SourceGroup        = "group"
	SourceDefault      = "default"
)

type ProfileLookup func(id uint) (*models.DeploymentProfile, error)

type Effective struct {
	ClientProfile    *models.DeploymentProfile `json:"client_profile,omitempty"`
	GroupProfile     *models.DeploymentProfile `json:"group_profile,omitempty"`
	ImageFilename    string                    `json:"image_filename,omitempty"`
	AutoInstallFile  string                    `json:"auto_install_file,omitempty"`
	SANTargetID      *uint                     `json:"san_target_id,omitempty"`
	BootFromSAN      bool                      `json:"boot_from_san"`
	BootloaderSet    string                    `json:"bootloader_set,omitempty"`
	ShowPublicImages bool                      `json:"show_public_images"`
	FirstBootAction  string                    `json:"first_boot_action,omitempty"`
	DiskImageID      *uint                     `json:"disk_image_id,omitempty"`
	Sources          map[string]string         `json:"sources"`
}

func (e Effective) ActiveProfile() *models.DeploymentProfile {
	if e.ClientProfile != nil {
		return e.ClientProfile
	}
	return e.GroupProfile
}

func (e Effective) AutoInstallCandidates() []Candidate {
	var out []Candidate
	if e.ClientProfile != nil && e.ClientProfile.AutoInstallFile != "" {
		out = append(out, Candidate{Path: e.ClientProfile.AutoInstallFile, Source: "profile:" + e.ClientProfile.Name})
	}
	if e.GroupProfile != nil && e.GroupProfile.AutoInstallFile != "" {
		out = append(out, Candidate{Path: e.GroupProfile.AutoInstallFile, Source: "group-profile:" + e.GroupProfile.Name})
	}
	return out
}

type Candidate struct {
	Path   string
	Source string
}

func loadProfile(id *uint, lookup ProfileLookup) *models.DeploymentProfile {
	if id == nil || *id == 0 || lookup == nil {
		return nil
	}
	profile, err := lookup(*id)
	if err != nil {
		return nil
	}
	return profile
}

func Resolve(client *models.Client, group *models.ClientGroup, lookup ProfileLookup) Effective {
	eff := Effective{ShowPublicImages: true, Sources: map[string]string{}}
	if client != nil {
		eff.ClientProfile = loadProfile(client.ProfileID, lookup)
	}
	if group != nil {
		eff.GroupProfile = loadProfile(group.ProfileID, lookup)
	}

	eff.ImageFilename, eff.Sources["image_filename"] = firstString(
		profileString(eff.ClientProfile, func(p *models.DeploymentProfile) string { return p.ImageFilename }, SourceProfile),
		profileString(eff.GroupProfile, func(p *models.DeploymentProfile) string { return p.ImageFilename }, SourceGroupProfile),
	)

	clientAutoInstall := ""
	if client != nil {
		clientAutoInstall = client.AutoInstallFile
	}
	groupAutoInstall := ""
	if group != nil {
		groupAutoInstall = group.AutoInstallFile
	}
	eff.AutoInstallFile, eff.Sources["auto_install_file"] = firstString(
		valued{clientAutoInstall, SourceClient},
		profileString(eff.ClientProfile, func(p *models.DeploymentProfile) string { return p.AutoInstallFile }, SourceProfile),
		profileString(eff.GroupProfile, func(p *models.DeploymentProfile) string { return p.AutoInstallFile }, SourceGroupProfile),
		valued{groupAutoInstall, SourceGroup},
	)

	clientBootloader := ""
	if client != nil {
		clientBootloader = client.BootloaderSet
	}
	groupBootloader := ""
	if group != nil {
		groupBootloader = group.BootloaderSet
	}
	eff.BootloaderSet, eff.Sources["bootloader_set"] = firstString(
		valued{clientBootloader, SourceClient},
		profileString(eff.ClientProfile, func(p *models.DeploymentProfile) string { return p.BootloaderSet }, SourceProfile),
		profileString(eff.GroupProfile, func(p *models.DeploymentProfile) string { return p.BootloaderSet }, SourceGroupProfile),
		valued{groupBootloader, SourceGroup},
	)

	eff.SANTargetID, eff.BootFromSAN, eff.Sources["san_target_id"] = resolveSAN(client, group, eff.ClientProfile, eff.GroupProfile)

	eff.ShowPublicImages, eff.Sources["show_public_images"] = resolveShowPublic(client, eff.ClientProfile, eff.GroupProfile)

	switch {
	case eff.ClientProfile != nil && eff.ClientProfile.FirstBootAction != "":
		eff.FirstBootAction = eff.ClientProfile.FirstBootAction
		eff.DiskImageID = eff.ClientProfile.DiskImageID
		eff.Sources["first_boot_action"] = SourceProfile
	case eff.GroupProfile != nil && eff.GroupProfile.FirstBootAction != "":
		eff.FirstBootAction = eff.GroupProfile.FirstBootAction
		eff.DiskImageID = eff.GroupProfile.DiskImageID
		eff.Sources["first_boot_action"] = SourceGroupProfile
	default:
		eff.Sources["first_boot_action"] = SourceDefault
	}
	if eff.FirstBootAction != models.FirstBootActionDeploy {
		eff.DiskImageID = nil
	}
	return eff
}

type valued struct {
	value  string
	source string
}

func profileString(p *models.DeploymentProfile, get func(*models.DeploymentProfile) string, source string) valued {
	if p == nil {
		return valued{}
	}
	return valued{get(p), source}
}

func firstString(candidates ...valued) (string, string) {
	for _, c := range candidates {
		if c.value != "" {
			return c.value, c.source
		}
	}
	return "", SourceDefault
}

func resolveSAN(client *models.Client, group *models.ClientGroup, clientProfile, groupProfile *models.DeploymentProfile) (*uint, bool, string) {
	bootDefault := false
	if client != nil && client.BootFromSAN {
		bootDefault = true
	}
	if group != nil && group.BootFromSAN {
		bootDefault = true
	}
	if clientProfile != nil && clientProfile.BootFromSAN {
		bootDefault = true
	}
	if groupProfile != nil && groupProfile.BootFromSAN {
		bootDefault = true
	}
	if client != nil && client.SANTargetID != nil && *client.SANTargetID != 0 {
		return client.SANTargetID, bootDefault, SourceClient
	}
	if clientProfile != nil && clientProfile.SANTargetID != nil && *clientProfile.SANTargetID != 0 {
		return clientProfile.SANTargetID, bootDefault, SourceProfile
	}
	if groupProfile != nil && groupProfile.SANTargetID != nil && *groupProfile.SANTargetID != 0 {
		return groupProfile.SANTargetID, bootDefault, SourceGroupProfile
	}
	if group != nil && group.SANTargetID != nil && *group.SANTargetID != 0 {
		return group.SANTargetID, bootDefault, SourceGroup
	}
	return nil, false, SourceDefault
}

func resolveShowPublic(client *models.Client, clientProfile, groupProfile *models.DeploymentProfile) (bool, string) {
	if client != nil && !client.ShowPublicImages {
		return false, SourceClient
	}
	if clientProfile != nil && clientProfile.ShowPublicImages != nil {
		return *clientProfile.ShowPublicImages, SourceProfile
	}
	if groupProfile != nil && groupProfile.ShowPublicImages != nil {
		return *groupProfile.ShowPublicImages, SourceGroupProfile
	}
	if client != nil {
		return client.ShowPublicImages, SourceClient
	}
	return true, SourceDefault
}

func InstallCompletedFor(records []models.InstallRecord, imageFilename string) bool {
	for _, rec := range records {
		if rec.ImageFilename == imageFilename && rec.State == models.InstallStateCompleted {
			return true
		}
	}
	return false
}

func DeployJobExistsFor(jobs []*models.DiskJob, diskImageID uint) bool {
	for _, job := range jobs {
		if job.Kind == models.DiskJobKindDeploy && job.DiskImageID == diskImageID {
			return true
		}
	}
	return false
}
