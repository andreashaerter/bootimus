//go:build !windows

package diskimage

import (
	"os"
	"path/filepath"
	"testing"

	"bootimus/internal/models"
)

func TestDeleteImageKeepsRecordWhenFilesCannotBeRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires Unix file permissions and a non-root user")
	}
	manager, store, dir := newManager(t, true, clonezillaTool())
	image := &models.DiskImage{Name: "locked", State: models.DiskImageStateReady}
	if err := store.CreateDiskImage(image); err != nil {
		t.Fatal(err)
	}
	lockedDir := filepath.Join(dir, DirName, image.Name, "locked")
	if err := os.MkdirAll(lockedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockedDir, "disk.img"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedDir, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(lockedDir, 0755)
	if err := manager.DeleteImage(image.ID); err == nil {
		t.Fatal("expected file removal to fail")
	}
	if _, err := store.GetDiskImage(image.ID); err != nil {
		t.Fatalf("failed removal lost the image record: %v", err)
	}
	if err := os.Chmod(lockedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteImage(image.ID); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}
