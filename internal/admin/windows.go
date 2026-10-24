package admin

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/wim"
)

func (h *Handler) RebuildBootWim(imageID uint) error {
	var images []*models.Image
	images, err := h.storage.ListImages()
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	var image *models.Image
	for _, img := range images {
		if img.ID == imageID {
			image = img
			break
		}
	}

	if image == nil {
		return fmt.Errorf("image not found")
	}

	imageName := strings.TrimSuffix(image.Filename, filepath.Ext(image.Filename))
	imageDir := filepath.Join(h.isoDir, imageName)
	bootWimPath := filepath.Join(imageDir, "iso", "sources", "boot.wim")

	if _, err := os.Stat(bootWimPath); os.IsNotExist(err) {
		return fmt.Errorf("boot.wim not found at %s", bootWimPath)
	}

	driverPacks, err := h.storage.ListDriverPacksByImage(imageID)
	if err != nil {
		return fmt.Errorf("failed to list driver packs: %w", err)
	}

	if len(driverPacks) == 0 {
		log.Printf("No driver packs enabled for image %s, skipping rebuild", imageName)
		return nil
	}

	log.Printf("Rebuilding boot.wim for %s with %d driver pack(s)", imageName, len(driverPacks))

	mu := h.bootWimLock(image.Filename)
	mu.Lock()
	defer mu.Unlock()

	tempDir, err := os.MkdirTemp("", "bootimus-wim-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	extractDir := filepath.Join(tempDir, "extracted")
	driversDir := filepath.Join(tempDir, "drivers")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("failed to create extract directory: %w", err)
	}
	if err := os.MkdirAll(driversDir, 0755); err != nil {
		return fmt.Errorf("failed to create drivers directory: %w", err)
	}

	origPath := bootWimPath + ".orig"
	legacyBackupPath := bootWimPath + ".backup"
	if _, err := os.Stat(origPath); os.IsNotExist(err) && !image.SMBInstallEnabled {
		if _, err := os.Stat(legacyBackupPath); err == nil {
			log.Printf("Adopting legacy driver backup %s as the pristine boot.wim", legacyBackupPath)
			if err := os.Rename(legacyBackupPath, origPath); err != nil {
				return fmt.Errorf("failed to adopt legacy boot.wim backup: %w", err)
			}
		}
	}
	pristineBase := false
	if _, err := os.Stat(origPath); err == nil {
		log.Printf("Restoring pristine boot.wim from %s before applying drivers", origPath)
		if err := copyFile(origPath, bootWimPath); err != nil {
			return fmt.Errorf("failed to restore pristine boot.wim: %w", err)
		}
		pristineBase = true
	} else if !image.SMBInstallEnabled {
		log.Printf("Backing up pristine boot.wim to %s", origPath)
		if err := copyFile(bootWimPath, origPath); err != nil {
			return fmt.Errorf("failed to back up pristine boot.wim: %w", err)
		}
		pristineBase = true
	} else {
		log.Printf("No pristine backup of boot.wim for %s; applying drivers in place - re-extract the image to enable clean rebuilds", imageName)
	}
	os.Remove(legacyBackupPath)

	log.Printf("Extracting driver packs...")
	for _, pack := range driverPacks {
		zipPath := filepath.Join(imageDir, "drivers", pack.Filename)
		log.Printf("  - Extracting %s", pack.Filename)
		if err := extractZipFile(zipPath, driversDir); err != nil {
			return fmt.Errorf("failed to extract driver pack %s: %w", pack.Filename, err)
		}
	}

	log.Printf("Listing WIM images...")

	wimManager, err := wim.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create WIM manager: %w", err)
	}

	imageCount, err := wimManager.GetImageCount(bootWimPath)
	if err != nil {
		return fmt.Errorf("failed to get WIM image count: %w", err)
	}

	log.Printf("Processing %d WIM image(s)", imageCount)

	for idx := 1; idx <= imageCount; idx++ {
		log.Printf("Processing WIM image %d...", idx)

		log.Printf("  Updating image %d...", idx)
		extractCmd := exec.Command("wimupdate", bootWimPath, fmt.Sprintf("%d", idx))
		extractCmd.Stdin = strings.NewReader(fmt.Sprintf("add \"%s\" \"/drivers\"\n", driversDir))
		if output, err := extractCmd.CombinedOutput(); err != nil {
			log.Printf("wimupdate output: %s", string(output))
			return fmt.Errorf("failed to update WIM image %d: %w", idx, err)
		}
	}

	now := time.Now()
	for _, pack := range driverPacks {
		pack.LastApplied = &now
		if err := h.storage.UpdateDriverPack(pack.ID, pack); err != nil {
			log.Printf("Warning: Failed to update driver pack %d LastApplied: %v", pack.ID, err)
		}
	}

	if image.SMBInstallEnabled && pristineBase {
		if h.patchWindowsBootWim(image.Filename) {
			image.SMBPatchFingerprint = h.computeSMBPatchFingerprint(image)
			log.Printf("Re-applied SMB patch to rebuilt boot.wim for %s", imageName)
		} else {
			image.SMBInstallEnabled = false
			image.SMBPatchFingerprint = ""
			log.Printf("Warning: failed to re-apply SMB patch to rebuilt boot.wim for %s", imageName)
		}
		if err := h.storage.UpdateImage(image.Filename, image); err != nil {
			log.Printf("Warning: failed to update image %s after SMB re-patch: %v", imageName, err)
		}
	}

	log.Printf("Successfully rebuilt boot.wim for %s", imageName)
	return nil
}

func extractZipFile(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		filePath := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(filePath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in ZIP: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		outFile, err := os.Create(filePath)
		if err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}
