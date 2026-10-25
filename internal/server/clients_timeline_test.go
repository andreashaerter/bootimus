package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"bootimus/internal/models"
)

func timelineResponse(t *testing.T, env *accessEnv, path, token string) timelinePage {
	t.Helper()
	code, body := env.do(http.MethodGet, path, token)
	if code != http.StatusOK {
		t.Fatalf("GET %s: want 200, got %d %s", path, code, body)
	}
	var resp struct {
		Success bool         `json:"success"`
		Data    timelinePage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if !resp.Success {
		t.Fatalf("GET %s: success=false: %s", path, body)
	}
	return resp.Data
}

func seedTimeline(t *testing.T, env *accessEnv, mac string) {
	t.Helper()
	for i := 0; i < 3; i++ {
		if err := env.store.LogBootAttempt(mac, "Ubuntu", "10.0.0.5", i != 1, ""); err != nil {
			t.Fatal(err)
		}
	}
	finished := time.Now().Add(-time.Minute)
	started := finished.Add(-10 * time.Minute)
	if err := env.store.CreateInstallRecord(&models.InstallRecord{MACAddress: mac, ImageFilename: "ubuntu.iso", ImageName: "Ubuntu", Token: "tok1", State: models.InstallStateCompleted, BootedAt: started.Add(-time.Minute), InstallingAt: &started, FinishedAt: &finished}); err != nil {
		t.Fatal(err)
	}
	img := &models.DiskImage{Name: "lab-gold", Disk: "sda", State: "ready"}
	if err := env.store.CreateDiskImage(img); err != nil {
		t.Fatal(err)
	}
	if err := env.store.CreateDiskJob(&models.DiskJob{ClientMAC: mac, Kind: "deploy", DiskImageID: &img.ID, Disk: "sda", State: models.DiskJobStateFailed, Token: "job1", Message: "nfs_mount_failed"}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.CreateAuditLog(&models.AuditLog{Username: "admin", Method: "PUT", Path: "/api/clients", Query: "mac=" + mac, Status: 200, Summary: "Updated client " + mac}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.CreateAuditLog(&models.AuditLog{Username: "admin", Method: "PUT", Path: "/api/clients", Query: "mac=ff:ff:ff:ff:ff:ff", Status: 200, Summary: "Updated client ff:ff:ff:ff:ff:ff"}); err != nil {
		t.Fatal(err)
	}
}

func TestClientTimelineMergesSourcesNewestFirst(t *testing.T) {
	env := newAccessEnv(t)
	mac := "aa:bb:cc:dd:ee:10"
	seedTimeline(t, env, mac)

	page := timelineResponse(t, env, "/api/clients/timeline?mac="+mac, env.admin)
	kinds := map[string]int{}
	for i, it := range page.Items {
		kinds[it.Kind]++
		if i > 0 && it.Time.After(page.Items[i-1].Time) {
			t.Fatalf("items not newest first at %d: %v after %v", i, it.Time, page.Items[i-1].Time)
		}
	}
	if kinds[timelineKindBoot] != 3 || kinds[timelineKindInstall] != 3 || kinds[timelineKindDisk] != 1 || kinds[timelineKindAudit] != 1 {
		t.Fatalf("unexpected kind counts: %v", kinds)
	}
	if page.HasMore {
		t.Fatalf("page should not report more items: %+v", page)
	}
	for _, it := range page.Items {
		if it.Kind == timelineKindAudit && it.Title != "Updated client "+mac {
			t.Fatalf("audit entry for another client leaked: %+v", it)
		}
	}
}

func TestClientTimelinePaginatesByCursor(t *testing.T) {
	env := newAccessEnv(t)
	mac := "aa:bb:cc:dd:ee:11"
	seedTimeline(t, env, mac)

	all := timelineResponse(t, env, "/api/clients/timeline?mac="+mac, env.admin)
	first := timelineResponse(t, env, "/api/clients/timeline?mac="+mac+"&limit=3", env.admin)
	if len(first.Items) != 3 || !first.HasMore || first.NextBefore == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := timelineResponse(t, env, "/api/clients/timeline?mac="+mac+"&limit=100&before="+first.NextBefore, env.admin)
	if len(first.Items)+len(second.Items) != len(all.Items) {
		t.Fatalf("pages do not add up: %d + %d != %d", len(first.Items), len(second.Items), len(all.Items))
	}
	for _, it := range second.Items {
		if !it.Time.Before(first.Items[2].Time) {
			t.Fatalf("second page item %v is not before cursor %v", it.Time, first.Items[2].Time)
		}
	}
	if code, _ := env.do(http.MethodGet, "/api/clients/timeline?mac="+mac+"&before=yesterday", env.admin); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: want 400, got %d", code)
	}
}

func TestClientTimelineHidesAuditFromViewers(t *testing.T) {
	env := newAccessEnv(t)
	mac := "aa:bb:cc:dd:ee:12"
	seedTimeline(t, env, mac)

	page := timelineResponse(t, env, "/api/clients/timeline?mac="+mac, env.viewer)
	if len(page.Items) == 0 {
		t.Fatal("viewer should still see boot, install and disk items")
	}
	for _, it := range page.Items {
		if it.Kind == timelineKindAudit {
			t.Fatalf("viewer received an audit item: %+v", it)
		}
	}
	if code, _ := env.do(http.MethodGet, "/api/clients/timeline", env.admin); code != http.StatusBadRequest {
		t.Fatalf("missing mac: want 400, got %d", code)
	}
}
