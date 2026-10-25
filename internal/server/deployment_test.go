package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"bootimus/internal/autoinstall"
	"bootimus/internal/deployment"
	"bootimus/internal/models"
)

func createProfile(t *testing.T, env *accessEnv, p *models.DeploymentProfile) *models.DeploymentProfile {
	t.Helper()
	if err := env.store.CreateDeploymentProfile(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func createClient(t *testing.T, env *accessEnv, c *models.Client) *models.Client {
	t.Helper()
	if err := env.store.CreateClient(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDeploymentProfileOffersImageAndMakesInstallDefault(t *testing.T) {
	env := newAccessEnv(t)
	lab := &models.Image{Name: "Lab Ubuntu", Filename: "lab.iso", Enabled: true, Public: false}
	pub := &models.Image{Name: "Public Rescue", Filename: "rescue.iso", Enabled: true, Public: true}
	for _, img := range []*models.Image{lab, pub} {
		if err := env.store.CreateImage(img); err != nil {
			t.Fatal(err)
		}
	}
	profile := createProfile(t, env, &models.DeploymentProfile{Name: "lab", ImageFilename: "lab.iso", FirstBootAction: models.FirstBootActionInstall})
	client := createClient(t, env, &models.Client{MACAddress: "aa:bb:cc:dd:ee:21", Name: "lab-21", Enabled: true, ShowPublicImages: true, Static: true, ProfileID: &profile.ID})

	data := previewResponse(t, env, "/api/clients/menu-preview?mac="+client.MACAddress)
	if data.Profile != "lab" {
		t.Errorf("preview profile = %q, want lab", data.Profile)
	}
	item := fmt.Sprintf("item iso%d Lab Ubuntu", lab.ID)
	if !strings.Contains(data.Script, item) {
		t.Errorf("profile image not offered:\n%s", data.Script)
	}
	if !strings.Contains(data.Script, fmt.Sprintf("choose --default iso%d ", lab.ID)) {
		t.Errorf("profile install image should be the default item:\n%s", data.Script)
	}

	if err := env.store.SetNextBootImage(client.MACAddress, "rescue.iso"); err != nil {
		t.Fatal(err)
	}
	data = previewResponse(t, env, "/api/clients/menu-preview?mac="+client.MACAddress)
	if !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(data.Script, "#!ipxe")), fmt.Sprintf("goto iso%d", pub.ID)) {
		t.Errorf("a next-boot image must win over the profile default:\n%s", data.Script)
	}
	if err := env.store.ClearNextBootImage(client.MACAddress); err != nil {
		t.Fatal(err)
	}

	rec := &models.InstallRecord{MACAddress: client.MACAddress, ImageFilename: "lab.iso", ImageName: "Lab Ubuntu", Token: "tok-21", State: models.InstallStateCompleted, BootedAt: time.Now()}
	if err := env.store.CreateInstallRecord(rec); err != nil {
		t.Fatal(err)
	}
	data = previewResponse(t, env, "/api/clients/menu-preview?mac="+client.MACAddress)
	if !strings.Contains(data.Script, item) {
		t.Errorf("profile image must stay offered after the install:\n%s", data.Script)
	}
	if strings.Contains(data.Script, fmt.Sprintf("choose --default iso%d ", lab.ID)) {
		t.Errorf("a completed install must stop the profile image being the default:\n%s", data.Script)
	}

}

func TestDeploymentProfileSANAndHiddenPublicImages(t *testing.T) {
	env := newAccessEnv(t)
	pub := &models.Image{Name: "Public Rescue", Filename: "rescue.iso", Enabled: true, Public: true}
	if err := env.store.CreateImage(pub); err != nil {
		t.Fatal(err)
	}
	target := &models.SANTarget{Name: "lab-disk", URI: "iscsi:10.0.0.5::::iqn.2024-01.lan:disk0"}
	if err := env.store.CreateSANTarget(target); err != nil {
		t.Fatal(err)
	}
	hide := false
	profile := createProfile(t, env, &models.DeploymentProfile{Name: "diskless", SANTargetID: &target.ID, BootFromSAN: true, ShowPublicImages: &hide})
	group := &models.ClientGroup{Name: "kiosks", Enabled: true, ProfileID: &profile.ID}
	if err := env.store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	client := createClient(t, env, &models.Client{MACAddress: "aa:bb:cc:dd:ee:22", Enabled: true, ShowPublicImages: true, Static: true, ClientGroupID: &group.ID})

	data := previewResponse(t, env, "/api/clients/menu-preview?mac="+client.MACAddress)
	if !strings.Contains(data.Script, "item san Boot from SAN: lab-disk") || !strings.Contains(data.Script, "choose --default san ") {
		t.Errorf("group profile SAN target should be offered and default:\n%s", data.Script)
	}
	if strings.Contains(data.Script, fmt.Sprintf("item iso%d", pub.ID)) {
		t.Errorf("profile hides public images, but one is listed:\n%s", data.Script)
	}
}

func TestDeploymentProfileAutoInstallPrecedence(t *testing.T) {
	env := newAccessEnv(t)
	lib, err := autoinstall.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"client", "profile", "group-profile", "group", "image"} {
		if err := lib.Write("ubuntu", name+".yaml", name); err != nil {
			t.Fatal(err)
		}
	}
	env.server.autoInstallLib = lib
	image := &models.Image{Filename: "ubuntu.iso", AutoInstallFile: "ubuntu/image.yaml"}
	clientProfile := createProfile(t, env, &models.DeploymentProfile{Name: "cp", AutoInstallFile: "ubuntu/profile.yaml"})
	groupProfile := createProfile(t, env, &models.DeploymentProfile{Name: "gp", AutoInstallFile: "ubuntu/group-profile.yaml"})
	group := &models.ClientGroup{Name: "g", Enabled: true, AutoInstallFile: "ubuntu/group.yaml", ProfileID: &groupProfile.ID}
	if err := env.store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		client *models.Client
		want   string
	}{
		{"client file first", &models.Client{MACAddress: "aa:bb:cc:dd:ee:31", AutoInstallFile: "ubuntu/client.yaml", ProfileID: &clientProfile.ID, ClientGroupID: &group.ID}, "client"},
		{"client profile before group profile", &models.Client{MACAddress: "aa:bb:cc:dd:ee:32", ProfileID: &clientProfile.ID, ClientGroupID: &group.ID}, "profile"},
		{"group profile before group file", &models.Client{MACAddress: "aa:bb:cc:dd:ee:33", ClientGroupID: &group.ID}, "group-profile"},
		{"image last", &models.Client{MACAddress: "aa:bb:cc:dd:ee:34"}, "image"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content, _, _, err := env.server.resolveAutoInstallScript(image, tc.client)
			if err != nil {
				t.Fatal(err)
			}
			if content != tc.want {
				t.Errorf("resolved %q, want %q", content, tc.want)
			}
		})
	}
}

func TestDeploymentProfileBootloaderSet(t *testing.T) {
	env := newAccessEnv(t)
	profile := createProfile(t, env, &models.DeploymentProfile{Name: "legacy", BootloaderSet: "legacy-set"})
	group := &models.ClientGroup{Name: "old-kit", Enabled: true, BootloaderSet: "group-set", ProfileID: &profile.ID}
	if err := env.store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	client := createClient(t, env, &models.Client{MACAddress: "aa:bb:cc:dd:ee:41", Enabled: true, ShowPublicImages: true, Static: true, ClientGroupID: &group.ID})
	hw, _ := net.ParseMAC(client.MACAddress)
	if set, overridden := env.server.effectiveBootloaderSet(hw); !overridden || set != "legacy-set" {
		t.Errorf("profile bootloader set should win over the group's: %q %v", set, overridden)
	}
	client.BootloaderSet = "client-set"
	if err := env.store.UpdateClient(client.MACAddress, client); err != nil {
		t.Fatal(err)
	}
	if set, _ := env.server.effectiveBootloaderSet(hw); set != "client-set" {
		t.Errorf("client's own bootloader set should win: %q", set)
	}
}

func TestDeploymentProfileAPIAndEffective(t *testing.T) {
	env := newAccessEnv(t)
	if err := env.store.CreateImage(&models.Image{Name: "Ubuntu", Filename: "ubuntu.iso", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	code, body := env.doJSON(http.MethodPost, "/api/deployment-profiles", env.admin, map[string]interface{}{"name": "zero-touch", "image_filename": "missing.iso"})
	if code != http.StatusBadRequest {
		t.Fatalf("unknown image: want 400, got %d %s", code, body)
	}
	code, body = env.doJSON(http.MethodPost, "/api/deployment-profiles", env.admin, map[string]interface{}{"name": "zero-touch", "image_filename": "ubuntu.iso", "first_boot_action": "install", "bootloader_set": "custom"})
	if code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d %s", code, body)
	}
	var created struct {
		Data models.DeploymentProfile `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if code, _ := env.doJSON(http.MethodPost, "/api/deployment-profiles", env.admin, map[string]interface{}{"name": "zero-touch"}); code != http.StatusConflict {
		t.Errorf("duplicate name: want 409, got %d", code)
	}
	if code, _ := env.doJSON(http.MethodPost, "/api/deployment-profiles", env.viewer, map[string]interface{}{"name": "x"}); code != http.StatusForbidden {
		t.Errorf("viewer create: want 403, got %d", code)
	}
	code, body = env.do(http.MethodGet, "/api/deployment-profiles", env.viewer)
	if code != http.StatusOK || !strings.Contains(body, "zero-touch") {
		t.Errorf("viewer list: want 200 with the profile, got %d %s", code, body)
	}

	client := createClient(t, env, &models.Client{MACAddress: "aa:bb:cc:dd:ee:51", Name: "box", Enabled: true, ShowPublicImages: true, Static: true})
	if code, body := env.doJSON(http.MethodPut, "/api/clients?mac="+client.MACAddress, env.admin, map[string]interface{}{"profile_id": 999}); code != http.StatusBadRequest {
		t.Errorf("unknown profile on client: want 400, got %d %s", code, body)
	}
	if code, body := env.doJSON(http.MethodPut, "/api/clients?mac="+client.MACAddress, env.admin, map[string]interface{}{"profile_id": created.Data.ID}); code != http.StatusOK {
		t.Fatalf("assign profile: want 200, got %d %s", code, body)
	}

	code, body = env.do(http.MethodGet, "/api/clients/effective?mac="+client.MACAddress, env.viewer)
	if code != http.StatusOK {
		t.Fatalf("effective: want 200, got %d %s", code, body)
	}
	var eff struct {
		Data struct {
			deployment.Effective
			ProfileName string `json:"profile_name"`
			ImageName   string `json:"image_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &eff); err != nil {
		t.Fatal(err)
	}
	if eff.Data.ProfileName != "zero-touch" || eff.Data.ImageName != "Ubuntu" || eff.Data.BootloaderSet != "custom" || eff.Data.Sources["bootloader_set"] != deployment.SourceProfile || eff.Data.FirstBootAction != models.FirstBootActionInstall {
		t.Errorf("unexpected effective view: %+v", eff.Data)
	}

	code, body = env.doJSON(http.MethodPut, fmt.Sprintf("/api/deployment-profiles?id=%d", created.Data.ID), env.admin, map[string]interface{}{"name": "zero-touch", "description": "renamed"})
	if code != http.StatusOK {
		t.Fatalf("update: want 200, got %d %s", code, body)
	}
	if code, body := env.do(http.MethodDelete, fmt.Sprintf("/api/deployment-profiles?id=%d", created.Data.ID), env.admin); code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d %s", code, body)
	}
	if c, _ := env.store.GetClient(client.MACAddress); c.ProfileID != nil {
		t.Errorf("deleting the profile should clear it from the client")
	}
	if code, _ := env.do(http.MethodGet, "/api/clients/effective?mac=00:00:00:00:00:00", env.admin); code != http.StatusNotFound {
		t.Errorf("effective for unknown client: want 404, got %d", code)
	}
}

func TestEnsureFirstBootDeployWithoutDiskImaging(t *testing.T) {
	env := newAccessEnv(t)
	id := uint(7)
	eff := deployment.Effective{FirstBootAction: models.FirstBootActionDeploy, DiskImageID: &id}
	env.server.ensureFirstBootDeploy(&models.Client{MACAddress: "aa:bb:cc:dd:ee:61"}, eff)
}
