package storage

import (
	"errors"
	"testing"

	"bootimus/internal/models"
)

func TestDeleteSANTargetClearsDeploymentProfiles(t *testing.T) {
	store := newTestStore(t)
	target := &models.SANTarget{Name: "remove", URI: "aoe:e0.0"}
	if err := store.CreateSANTarget(target); err != nil {
		t.Fatal(err)
	}
	profile := &models.DeploymentProfile{Name: "san-profile", SANTargetID: &target.ID, BootFromSAN: true}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSANTarget(target.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetDeploymentProfile(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SANTargetID != nil || updated.BootFromSAN {
		t.Fatalf("deleted SAN target still referenced: %+v", updated)
	}
}

func TestDeleteDeploymentProfileClearsEnrolmentRules(t *testing.T) {
	store := newTestStore(t)
	profile := &models.DeploymentProfile{Name: "remove"}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		t.Fatal(err)
	}
	rules := []*models.EnrolmentRule{
		{Name: "profile only", ProfileID: &profile.ID, MACPrefix: "aa:bb", Enabled: true},
		{Name: "also names", ProfileID: &profile.ID, MACPrefix: "aa:cc", NamePattern: "node-{{MAC_SUFFIX}}", Enabled: true},
	}
	for _, rule := range rules {
		if err := store.CreateEnrolmentRule(rule); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteDeploymentProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	for index, rule := range rules {
		updated, err := store.GetEnrolmentRule(rule.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.ProfileID != nil {
			t.Errorf("deleted profile still referenced by %s", rule.Name)
		}
		if updated.Enabled != (index == 1) {
			t.Errorf("rule %s enabled=%v after removing its profile", rule.Name, updated.Enabled)
		}
	}
}

func TestDeleteDiskImageClearsProfilesAndRollsBackFileFailure(t *testing.T) {
	store := newTestStore(t)
	image := &models.DiskImage{Name: "remove", State: models.DiskImageStateReady}
	if err := store.CreateDiskImage(image); err != nil {
		t.Fatal(err)
	}
	profile := &models.DeploymentProfile{Name: "disk-profile", DiskImageID: &image.ID, FirstBootAction: models.FirstBootActionDeploy}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		t.Fatal(err)
	}
	removalErr := errors.New("cannot remove files")
	if err := store.DeleteDiskImage(image.ID, func() error { return removalErr }); !errors.Is(err, removalErr) {
		t.Fatalf("delete returned %v, want file error", err)
	}
	if _, err := store.GetDiskImage(image.ID); err != nil {
		t.Fatalf("file failure lost the image record: %v", err)
	}
	updated, err := store.GetDeploymentProfile(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DiskImageID == nil || *updated.DiskImageID != image.ID || updated.FirstBootAction != models.FirstBootActionDeploy {
		t.Fatal("file failure changed the profile")
	}
	if err := store.DeleteDiskImage(image.ID, nil); err != nil {
		t.Fatal(err)
	}
	updated, err = store.GetDeploymentProfile(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DiskImageID != nil || updated.FirstBootAction != models.FirstBootActionNone {
		t.Fatalf("deleted disk image still referenced: %+v", updated)
	}
}
