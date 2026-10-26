package server

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/proxydhcp"
)

func TestClientTimelinePaginatesEqualTimestamps(t *testing.T) {
	env := newAccessEnv(t)
	mac := "aa:bb:cc:dd:ee:71"
	if err := env.store.LogBootAttempt(mac, "Ubuntu", "192.0.2.1", true, ""); err != nil {
		t.Fatal(err)
	}
	logs, err := env.store.GetBootLogsByMAC(mac, 1)
	if err != nil || len(logs) != 1 {
		t.Fatalf("boot log fixture: %v %v", logs, err)
	}
	stamp := logs[0].CreatedAt
	for _, summary := range []string{"first", "second"} {
		if err := env.store.CreateAuditLog(&models.AuditLog{CreatedAt: stamp, Username: "admin", Method: "PUT", Path: "/api/clients", Query: "mac=" + mac, Status: 200, Summary: summary + " " + mac}); err != nil {
			t.Fatal(err)
		}
	}
	if err := env.store.CreateInstallRecord(&models.InstallRecord{MACAddress: mac, ImageFilename: "ubuntu.iso", ImageName: "Ubuntu", Token: "tied-install", State: models.InstallStateCompleted, BootedAt: stamp, InstallingAt: &stamp, FinishedAt: &stamp}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.CreateDiskJob(&models.DiskJob{ClientMAC: mac, Kind: models.DiskJobKindDeploy, State: models.DiskJobStateCompleted, Token: "tied-disk", FinishedAt: &stamp}); err != nil {
		t.Fatal(err)
	}
	env.server.proxyDHCPRecorder = proxydhcp.NewRecorder(10)
	for count := 0; count < 2; count++ {
		env.server.proxyDHCPRecorder.Record(proxydhcp.Event{Time: stamp, MAC: mac, MessageType: "DISCOVER", Source: "192.0.2.1:68", Outcome: proxydhcp.OutcomeReplied})
	}
	for _, role := range []struct {
		name  string
		token string
		count int
	}{{"admin", env.admin, 9}, {"viewer", env.viewer, 7}} {
		t.Run(role.name, func(t *testing.T) {
			all := timelineResponse(t, env, "/api/clients/timeline?mac="+mac, role.token)
			if len(all.Items) != role.count {
				t.Fatalf("fixture has %d items, want %d", len(all.Items), role.count)
			}
			for _, limit := range []int{1, 2, 4} {
				t.Run(strconv.Itoa(limit), func(t *testing.T) {
					var combined []timelineItem
					cursor := ""
					for pageNumber := 0; pageNumber <= len(all.Items); pageNumber++ {
						page := timelineResponse(t, env, "/api/clients/timeline?mac="+mac+"&limit="+strconv.Itoa(limit)+"&before="+url.QueryEscape(cursor), role.token)
						combined = append(combined, page.Items...)
						if !page.HasMore {
							if page.NextBefore != "" {
								t.Error("last page has a cursor")
							}
							break
						}
						if page.NextBefore == "" || page.NextBefore == cursor {
							t.Fatal("cursor did not advance")
						}
						cursor = page.NextBefore
					}
					if !reflect.DeepEqual(combined, all.Items) {
						t.Fatalf("pagination lost, duplicated or reordered events: got %+v, want %+v", combined, all.Items)
					}
				})
			}
		})
	}
}

func TestClientTimelineAcceptsLegacyTimeBounds(t *testing.T) {
	env := newAccessEnv(t)
	mac := "aa:bb:cc:dd:ee:72"
	stamp := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.FixedZone("test", 5400))
	for _, offset := range []time.Duration{-time.Second, 0, time.Second} {
		if err := env.store.CreateAuditLog(&models.AuditLog{CreatedAt: stamp.Add(offset), Summary: mac, Status: 200}); err != nil {
			t.Fatal(err)
		}
	}
	page := timelineResponse(t, env, "/api/clients/timeline?mac="+mac+"&before="+url.QueryEscape(stamp.Format(time.RFC3339Nano)), env.admin)
	if len(page.Items) != 1 || !page.Items[0].Time.Equal(stamp.Add(-time.Second)) {
		t.Fatalf("legacy time bound is not exclusive: %+v", page.Items)
	}
}

func TestClientTimelineCursorSurvivesDHCPEviction(t *testing.T) {
	env := newAccessEnv(t)
	mac := "aa:bb:cc:dd:ee:74"
	stamp := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	recorder := proxydhcp.NewRecorder(3)
	env.server.proxyDHCPRecorder = recorder
	for number := 1; number <= 3; number++ {
		recorder.Record(proxydhcp.Event{Time: stamp, MAC: mac, MessageType: strconv.Itoa(number)})
	}
	first := timelineResponse(t, env, "/api/clients/timeline?mac="+mac+"&limit=2", env.admin)
	if len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("first page = %+v", first)
	}
	recorder.Record(proxydhcp.Event{Time: stamp, MAC: mac, MessageType: "4"})
	second := timelineResponse(t, env, "/api/clients/timeline?mac="+mac+"&before="+url.QueryEscape(first.NextBefore), env.admin)
	if len(second.Items) != 2 || second.Items[0].Title != "proxyDHCP 3 from " || second.Items[1].Title != "proxyDHCP 4 from " {
		t.Fatalf("eviction changed cursor position: %+v", second.Items)
	}
}

func TestClientTimelineRejectsInvalidCursors(t *testing.T) {
	env := newAccessEnv(t)
	cursors := []string{"yesterday", "v2.e30", "v1.%", "v1." + strings.Repeat("A", 513)}
	for _, body := range []string{`{}`, `{"time":"not-a-time","key":"audit:1"}`, `{"time":"2026-10-20T12:00:00Z"}`, `{"time":"0001-01-01T00:00:00Z","key":"audit:1"}`} {
		cursors = append(cursors, "v1."+base64.RawURLEncoding.EncodeToString([]byte(body)))
	}
	for _, cursor := range cursors {
		if code, body := env.do(http.MethodGet, "/api/clients/timeline?mac=aa:bb:cc:dd:ee:73&before="+url.QueryEscape(cursor), env.admin); code != http.StatusBadRequest {
			t.Errorf("cursor %q: got %d %s, want 400", cursor, code, body)
		}
	}
}
