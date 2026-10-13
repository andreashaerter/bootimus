package server

import (
	"strings"
	"testing"

	"bootimus/internal/models"
)

func TestBuildKernelBootSectionDefault(t *testing.T) {
	mb := &MenuBuilder{serverAddr: "10.0.0.1", httpPort: 8080}
	img := models.Image{
		ID:           1,
		Name:         "Kali",
		Filename:     "kali-linux-2026.2-live-amd64.iso",
		Enabled:      true,
		BootMethod:   "kernel",
		Distro:       "kali",
		BootParams:   "initrd=initrd boot=live fetch={{SQUASHFS}}",
		SquashfsPath: "iso/LIVE/filesystem.squashfs",
	}

	section := mb.buildKernelBootSection(&img, "kali-linux-2026.2-live-amd64.iso", "kali-linux-2026.2-live-amd64")

	if !strings.Contains(section, "kernel http://10.0.0.1:8080/boot/kali-linux-2026.2-live-amd64/vmlinuz ") {
		t.Errorf("expected default kernel path, got:\n%s", section)
	}
	if !strings.Contains(section, "initrd http://10.0.0.1:8080/boot/kali-linux-2026.2-live-amd64/initrd\n") {
		t.Errorf("expected default initrd path, got:\n%s", section)
	}
	if !strings.Contains(section, "fetch=http://10.0.0.1:8080/boot/kali-linux-2026.2-live-amd64/iso/LIVE/filesystem.squashfs") {
		t.Errorf("expected squashfs fetch URL, got:\n%s", section)
	}
}

func TestBuildKernelBootSectionOverrides(t *testing.T) {
	mb := &MenuBuilder{serverAddr: "10.0.0.1", httpPort: 8080}
	img := models.Image{
		ID:             1,
		Name:           "Kali",
		Filename:       "kali-linux-2026.2-live-amd64.iso",
		Enabled:        true,
		BootMethod:     "kernel",
		Distro:         "kali",
		BootParams:     "initrd=initrd boot=live fetch={{SQUASHFS}}",
		SquashfsPath:   "iso/LIVE/filesystem.squashfs",
		KernelOverride: "iso/LIVE/vmlinuz-6.19.14+kali-amd64",
		InitrdOverride: "iso/LIVE/initrd.img-6.19.14+kali-amd64",
	}

	section := mb.buildKernelBootSection(&img, "kali-linux-2026.2-live-amd64.iso", "kali-linux-2026.2-live-amd64")

	if !strings.Contains(section, "kernel http://10.0.0.1:8080/boot/kali-linux-2026.2-live-amd64/iso/LIVE/vmlinuz-6.19.14+kali-amd64 ") {
		t.Errorf("expected overridden kernel path, got:\n%s", section)
	}
	if !strings.Contains(section, "initrd --name initrd http://10.0.0.1:8080/boot/kali-linux-2026.2-live-amd64/iso/LIVE/initrd.img-6.19.14+kali-amd64\n") {
		t.Errorf("expected overridden initrd path with explicit initrd name, got:\n%s", section)
	}
}
