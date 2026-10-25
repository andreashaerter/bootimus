package deployment

import (
	"errors"
	"testing"
	"time"

	"bootimus/internal/models"
)

func uintPtr(v uint) *uint { return &v }

func boolPtr(v bool) *bool { return &v }

func lookupFrom(profiles ...*models.DeploymentProfile) ProfileLookup {
	return func(id uint) (*models.DeploymentProfile, error) {
		for _, p := range profiles {
			if p.ID == id {
				return p, nil
			}
		}
		return nil, errors.New("not found")
	}
}

func TestResolvePrecedence(t *testing.T) {
	clientProfile := &models.DeploymentProfile{ID: 1, Name: "client-profile", ImageFilename: "cp.iso", AutoInstallFile: "cp/auto.yaml", BootloaderSet: "cp-set", SANTargetID: uintPtr(11), FirstBootAction: models.FirstBootActionInstall}
	groupProfile := &models.DeploymentProfile{ID: 2, Name: "group-profile", ImageFilename: "gp.iso", AutoInstallFile: "gp/auto.yaml", BootloaderSet: "gp-set", SANTargetID: uintPtr(22), BootFromSAN: true, FirstBootAction: models.FirstBootActionDeploy, DiskImageID: uintPtr(5)}
	lookup := lookupFrom(clientProfile, groupProfile)

	tests := []struct {
		name            string
		client          *models.Client
		group           *models.ClientGroup
		wantImage       string
		wantImageSource string
		wantAutoInstall string
		wantAutoSource  string
		wantBootloader  string
		wantBootSource  string
		wantSAN         *uint
		wantSANSource   string
		wantBootFromSAN bool
		wantFirstBoot   string
		wantDiskImage   *uint
	}{
		{
			name:            "no profiles keeps client and group values",
			client:          &models.Client{AutoInstallFile: "c/auto.yaml", BootloaderSet: "c-set", SANTargetID: uintPtr(1)},
			group:           &models.ClientGroup{AutoInstallFile: "g/auto.yaml", BootloaderSet: "g-set"},
			wantImageSource: SourceDefault,
			wantAutoInstall: "c/auto.yaml", wantAutoSource: SourceClient,
			wantBootloader: "c-set", wantBootSource: SourceClient,
			wantSAN: uintPtr(1), wantSANSource: SourceClient,
		},
		{
			name:            "group values apply when the client has none",
			client:          &models.Client{},
			group:           &models.ClientGroup{AutoInstallFile: "g/auto.yaml", BootloaderSet: "g-set", SANTargetID: uintPtr(3), BootFromSAN: true},
			wantImageSource: SourceDefault,
			wantAutoInstall: "g/auto.yaml", wantAutoSource: SourceGroup,
			wantBootloader: "g-set", wantBootSource: SourceGroup,
			wantSAN: uintPtr(3), wantSANSource: SourceGroup, wantBootFromSAN: true,
		},
		{
			name:      "client profile beats group values but not explicit client values",
			client:    &models.Client{ProfileID: uintPtr(1), BootloaderSet: "c-set"},
			group:     &models.ClientGroup{AutoInstallFile: "g/auto.yaml", BootloaderSet: "g-set", SANTargetID: uintPtr(3)},
			wantImage: "cp.iso", wantImageSource: SourceProfile,
			wantAutoInstall: "cp/auto.yaml", wantAutoSource: SourceProfile,
			wantBootloader: "c-set", wantBootSource: SourceClient,
			wantSAN: uintPtr(11), wantSANSource: SourceProfile,
			wantFirstBoot: models.FirstBootActionInstall,
		},
		{
			name:      "group profile applies when the client has no profile",
			client:    &models.Client{},
			group:     &models.ClientGroup{ProfileID: uintPtr(2), BootloaderSet: "g-set"},
			wantImage: "gp.iso", wantImageSource: SourceGroupProfile,
			wantAutoInstall: "gp/auto.yaml", wantAutoSource: SourceGroupProfile,
			wantBootloader: "gp-set", wantBootSource: SourceGroupProfile,
			wantSAN: uintPtr(22), wantSANSource: SourceGroupProfile, wantBootFromSAN: true,
			wantFirstBoot: models.FirstBootActionDeploy, wantDiskImage: uintPtr(5),
		},
		{
			name:      "client profile beats group profile",
			client:    &models.Client{ProfileID: uintPtr(1)},
			group:     &models.ClientGroup{ProfileID: uintPtr(2)},
			wantImage: "cp.iso", wantImageSource: SourceProfile,
			wantAutoInstall: "cp/auto.yaml", wantAutoSource: SourceProfile,
			wantBootloader: "cp-set", wantBootSource: SourceProfile,
			wantSAN: uintPtr(11), wantSANSource: SourceProfile, wantBootFromSAN: true,
			wantFirstBoot: models.FirstBootActionInstall,
		},
		{
			name:            "missing profile is ignored",
			client:          &models.Client{ProfileID: uintPtr(99), BootloaderSet: "c-set"},
			wantImageSource: SourceDefault,
			wantAutoSource:  SourceDefault,
			wantBootloader:  "c-set", wantBootSource: SourceClient,
			wantSANSource: SourceDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eff := Resolve(tt.client, tt.group, lookup)
			if eff.ImageFilename != tt.wantImage || eff.Sources["image_filename"] != tt.wantImageSource {
				t.Errorf("image = %q (%s), want %q (%s)", eff.ImageFilename, eff.Sources["image_filename"], tt.wantImage, tt.wantImageSource)
			}
			if eff.AutoInstallFile != tt.wantAutoInstall || eff.Sources["auto_install_file"] != tt.wantAutoSource {
				t.Errorf("auto-install = %q (%s), want %q (%s)", eff.AutoInstallFile, eff.Sources["auto_install_file"], tt.wantAutoInstall, tt.wantAutoSource)
			}
			if eff.BootloaderSet != tt.wantBootloader || eff.Sources["bootloader_set"] != tt.wantBootSource {
				t.Errorf("bootloader = %q (%s), want %q (%s)", eff.BootloaderSet, eff.Sources["bootloader_set"], tt.wantBootloader, tt.wantBootSource)
			}
			if (eff.SANTargetID == nil) != (tt.wantSAN == nil) || (eff.SANTargetID != nil && *eff.SANTargetID != *tt.wantSAN) || eff.Sources["san_target_id"] != tt.wantSANSource {
				t.Errorf("san = %v (%s), want %v (%s)", eff.SANTargetID, eff.Sources["san_target_id"], tt.wantSAN, tt.wantSANSource)
			}
			if eff.BootFromSAN != tt.wantBootFromSAN {
				t.Errorf("boot from SAN = %v, want %v", eff.BootFromSAN, tt.wantBootFromSAN)
			}
			if eff.FirstBootAction != tt.wantFirstBoot {
				t.Errorf("first boot = %q, want %q", eff.FirstBootAction, tt.wantFirstBoot)
			}
			if (eff.DiskImageID == nil) != (tt.wantDiskImage == nil) || (eff.DiskImageID != nil && *eff.DiskImageID != *tt.wantDiskImage) {
				t.Errorf("disk image = %v, want %v", eff.DiskImageID, tt.wantDiskImage)
			}
		})
	}
}

func TestResolveShowPublicImages(t *testing.T) {
	hide := &models.DeploymentProfile{ID: 1, Name: "hide", ShowPublicImages: boolPtr(false)}
	show := &models.DeploymentProfile{ID: 2, Name: "show", ShowPublicImages: boolPtr(true)}
	lookup := lookupFrom(hide, show)

	if eff := Resolve(&models.Client{ShowPublicImages: true, ProfileID: uintPtr(1)}, nil, lookup); eff.ShowPublicImages || eff.Sources["show_public_images"] != SourceProfile {
		t.Errorf("profile should hide public images: %+v", eff)
	}
	if eff := Resolve(&models.Client{ShowPublicImages: false, ProfileID: uintPtr(2)}, nil, lookup); eff.ShowPublicImages || eff.Sources["show_public_images"] != SourceClient {
		t.Errorf("a client with public images off keeps them off: %+v", eff)
	}
	if eff := Resolve(&models.Client{ShowPublicImages: true}, &models.ClientGroup{ProfileID: uintPtr(1)}, lookup); eff.ShowPublicImages || eff.Sources["show_public_images"] != SourceGroupProfile {
		t.Errorf("group profile should hide public images: %+v", eff)
	}
	if eff := Resolve(&models.Client{ShowPublicImages: true}, nil, lookup); !eff.ShowPublicImages {
		t.Errorf("default keeps public images visible: %+v", eff)
	}
}

func TestAutoInstallCandidatesOrder(t *testing.T) {
	eff := Effective{
		ClientProfile: &models.DeploymentProfile{Name: "cp", AutoInstallFile: "cp.yaml"},
		GroupProfile:  &models.DeploymentProfile{Name: "gp", AutoInstallFile: "gp.yaml"},
	}
	got := eff.AutoInstallCandidates()
	if len(got) != 2 || got[0].Path != "cp.yaml" || got[0].Source != "profile:cp" || got[1].Path != "gp.yaml" || got[1].Source != "group-profile:gp" {
		t.Fatalf("unexpected candidates: %+v", got)
	}
}

func TestInstallCompletedForAndDeployJobExists(t *testing.T) {
	now := time.Now()
	records := []models.InstallRecord{
		{ImageFilename: "a.iso", State: models.InstallStateFailed, BootedAt: now},
		{ImageFilename: "b.iso", State: models.InstallStateCompleted, BootedAt: now},
	}
	if InstallCompletedFor(records, "a.iso") {
		t.Error("a failed install must not count as completed")
	}
	if !InstallCompletedFor(records, "b.iso") {
		t.Error("expected b.iso to be completed")
	}
	jobs := []*models.DiskJob{{Kind: models.DiskJobKindCapture, DiskImageID: 7}, {Kind: models.DiskJobKindDeploy, DiskImageID: 8}}
	if DeployJobExistsFor(jobs, 7) {
		t.Error("a capture job is not a deploy job")
	}
	if !DeployJobExistsFor(jobs, 8) {
		t.Error("expected a deploy job for image 8")
	}
}
