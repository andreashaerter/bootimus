package replica

import (
	"context"
	"testing"

	"bootimus/internal/models"
)

func TestReplicaPreservesProtectionWithoutPublishingHashes(test *testing.T) {
	environment := newEnv(test, false)
	environment.primary.mu.Lock()
	environment.primary.data["/api/theme"] = models.MenuTheme{Title: "Protected", BootPasswordRequired: true, BootPasswordHash: "must-not-cross-the-read-API"}
	environment.primary.data["/api/images"] = []models.Image{{ID: 7, Name: "Protected", Filename: "protected.iso", Enabled: true, Public: true, BootPasswordRequired: true, BootPasswordHash: "must-not-cross-the-read-API"}}
	environment.primary.mu.Unlock()
	environment.syncer.RunNow(context.Background())
	theme, err := environment.store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	image, err := environment.store.GetImage("protected.iso")
	if err != nil {
		test.Fatal(err)
	}
	if !theme.BootPasswordRequired || !image.BootPasswordRequired {
		test.Fatal("replication discarded protection")
	}
	if theme.BootPasswordHash != "" || image.BootPasswordHash != "" {
		test.Fatal("password hashes leaked through read APIs")
	}
}
