package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
)

func TestGetImageMarksLegacySMBPatchesForRepatch(t *testing.T) {
	store := storagetest.New(t)
	handler := &Handler{storage: store, isoDir: t.TempDir(), serverAddr: "192.0.2.1"}
	image := &models.Image{Name: "Windows", Filename: "windows.iso", SMBInstallEnabled: true}
	legacyHash := sha256.New()
	fmt.Fprintf(legacyHash, "addr=%s\nfile=%s\nai_enabled=%v\nai_file=%s\nai_content=", handler.serverAddr, image.Filename, image.AutoInstallEnabled, image.AutoInstallFile)
	legacyFingerprint := hex.EncodeToString(legacyHash.Sum(nil))
	currentFingerprint := handler.computeSMBPatchFingerprint(image)
	if currentFingerprint == legacyFingerprint {
		t.Fatal("changed share names did not invalidate the patch fingerprint")
	}
	if err := store.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name        string
		fingerprint string
		repatch     bool
	}{
		{"missing fingerprint", "", true},
		{"legacy fingerprint", legacyFingerprint, true},
		{"current fingerprint", currentFingerprint, false},
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			image.SMBPatchFingerprint = testcase.fingerprint
			if err := store.UpdateImage(image.Filename, image); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			handler.GetImage(recorder, httptest.NewRequest(http.MethodGet, "/api/images?filename=windows.iso", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("get image: %d %s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Data models.Image `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data.SMBNeedsRepatch != testcase.repatch {
				t.Errorf("needs repatch=%v, want %v", response.Data.SMBNeedsRepatch, testcase.repatch)
			}
		})
	}
}
