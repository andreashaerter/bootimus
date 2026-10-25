package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
	"bootimus/internal/tools"
)

func TestDownloadToolAcceptsCustomTools(t *testing.T) {
	dataDir := t.TempDir()
	store := storagetest.New(t)
	tm := tools.NewManager(store, dataDir)

	origin := httptest.NewServer(http.NotFoundHandler())
	defer origin.Close()

	if err := store.SaveBootTool(&models.BootTool{
		Name:        "my-rescue",
		DisplayName: "My Rescue",
		Custom:      true,
		DownloadURL: origin.URL + "/my-rescue.efi",
		KernelPath:  "my-rescue.efi",
		ArchiveType: "bin",
		BootMethod:  "chain",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBootTool(&models.BootTool{Name: "stale", DisplayName: "Stale"}); err != nil {
		t.Fatal(err)
	}

	h := &Handler{storage: store, toolsManager: tm}

	cases := []struct {
		name string
		want int
	}{
		{"my-rescue", http.StatusOK},
		{"stale", http.StatusNotFound},
		{"missing", http.StatusNotFound},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.DownloadTool(rec, httptest.NewRequest(http.MethodPost, "/api/tools/download?name="+c.name, nil))
		if rec.Code != c.want {
			var resp Response
			json.Unmarshal(rec.Body.Bytes(), &resp)
			t.Errorf("DownloadTool(%s) = %d (%s), want %d", c.name, rec.Code, resp.Error, c.want)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for tm.InProgress("my-rescue") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}
