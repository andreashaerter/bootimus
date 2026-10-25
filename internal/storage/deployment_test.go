package storage

import (
	"errors"
	"testing"

	"bootimus/internal/models"
)

func TestDeploymentProfileRoundTripAndDeleteClearsReferences(t *testing.T) {
	store := newTestStore(t)
	san := &models.SANTarget{Name: "disk", URI: "iscsi:10.0.0.5::::iqn.2024-01.lan:disk0"}
	if err := store.CreateSANTarget(san); err != nil {
		t.Fatal(err)
	}
	show := false
	profile := &models.DeploymentProfile{Name: "lab", Description: "Lab machines", ImageFilename: "ubuntu.iso", AutoInstallFile: "ubuntu/lab.yaml", SANTargetID: &san.ID, BootFromSAN: true, BootloaderSet: "custom", FirstBootAction: models.FirstBootActionInstall, ShowPublicImages: &show}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDeploymentProfile(&models.DeploymentProfile{Name: "lab"}); !errors.Is(err, ErrDeploymentProfileExists) {
		t.Fatalf("duplicate name: err = %v, want ErrDeploymentProfileExists", err)
	}

	got, err := store.GetDeploymentProfile(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "lab" || got.ImageFilename != "ubuntu.iso" || got.SANTargetID == nil || *got.SANTargetID != san.ID || !got.BootFromSAN || got.FirstBootAction != models.FirstBootActionInstall || got.ShowPublicImages == nil || *got.ShowPublicImages {
		t.Fatalf("round trip lost fields: %+v", got)
	}

	got.Description = "Changed"
	got.BootFromSAN = false
	if err := store.UpdateDeploymentProfile(profile.ID, got); err != nil {
		t.Fatal(err)
	}
	updated, _ := store.GetDeploymentProfile(profile.ID)
	if updated.Description != "Changed" || updated.BootFromSAN {
		t.Fatalf("update not applied: %+v", updated)
	}

	client := &models.Client{MACAddress: "aa:bb:cc:dd:ee:10", Enabled: true, ShowPublicImages: true, Static: true, ProfileID: &profile.ID}
	if err := store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	group := &models.ClientGroup{Name: "lab-group", Enabled: true, ProfileID: &profile.ID}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	clients, groups, err := store.CountDeploymentProfileUsers(profile.ID)
	if err != nil || clients != 1 || groups != 1 {
		t.Fatalf("usage counts = %d clients, %d groups, err %v", clients, groups, err)
	}

	if err := store.DeleteDeploymentProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDeploymentProfile(profile.ID); err == nil {
		t.Fatal("profile still present after delete")
	}
	c, _ := store.GetClient(client.MACAddress)
	if c.ProfileID != nil {
		t.Errorf("client still references the deleted profile")
	}
	g, _ := store.GetClientGroup(group.ID)
	if g.ProfileID != nil {
		t.Errorf("group still references the deleted profile")
	}
}

func TestDeploymentProfileValidation(t *testing.T) {
	cases := []struct {
		name    string
		profile models.DeploymentProfile
		wantErr bool
	}{
		{"empty name", models.DeploymentProfile{}, true},
		{"install without image", models.DeploymentProfile{Name: "a", FirstBootAction: models.FirstBootActionInstall}, true},
		{"deploy without disk image", models.DeploymentProfile{Name: "a", FirstBootAction: models.FirstBootActionDeploy}, true},
		{"unknown action", models.DeploymentProfile{Name: "a", FirstBootAction: "wipe"}, true},
		{"boot from san without target", models.DeploymentProfile{Name: "a", BootFromSAN: true}, true},
		{"plain", models.DeploymentProfile{Name: "a"}, false},
		{"install with image", models.DeploymentProfile{Name: "a", FirstBootAction: models.FirstBootActionInstall, ImageFilename: "x.iso"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.profile
			if err := ValidateDeploymentProfile(&p); (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestClientAndGroupUpdatesPersistProfile(t *testing.T) {
	store := newTestStore(t)
	profile := &models.DeploymentProfile{Name: "p"}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		t.Fatal(err)
	}
	client := &models.Client{MACAddress: "aa:bb:cc:dd:ee:11", Enabled: true, ShowPublicImages: true, Static: true}
	if err := store.CreateClient(client); err != nil {
		t.Fatal(err)
	}
	client.ProfileID = &profile.ID
	if err := store.UpdateClient(client.MACAddress, client); err != nil {
		t.Fatal(err)
	}
	if c, _ := store.GetClient(client.MACAddress); c.ProfileID == nil || *c.ProfileID != profile.ID {
		t.Fatalf("client profile not saved: %+v", c)
	}
	group := &models.ClientGroup{Name: "g", Enabled: true}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	group.ProfileID = &profile.ID
	if err := store.UpdateClientGroup(group.ID, group); err != nil {
		t.Fatal(err)
	}
	if g, _ := store.GetClientGroup(group.ID); g.ProfileID == nil || *g.ProfileID != profile.ID {
		t.Fatalf("group profile not saved: %+v", g)
	}
}
