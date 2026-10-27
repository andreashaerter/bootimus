package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	macBIOSLocal = "52:54:00:e2:e0:04"
	macUEFILocal = "52:54:00:e2:e0:05"

	serialLocalBoot      = "Booting from local disk..."
	serialFirmwareReturn = "Returning to the firmware boot order..."
	serialLocalDiskMBR   = "Bootimus e2e boot sector running from the local disk"
)

func biosBootSectorPrinting(msg string) []byte {
	sector := make([]byte, 512)
	code := []byte{
		0xFA, 0x31, 0xC0, 0x8E, 0xD8, 0x8E, 0xC0, 0x8E, 0xD0, 0xBC, 0x00, 0x7C, 0xFB,
		0xBE, 0x21, 0x7C,
		0xAC, 0x08, 0xC0, 0x74, 0x09, 0xB4, 0x0E, 0xBB, 0x07, 0x00, 0xCD, 0x10, 0xEB, 0xF2,
		0xF4, 0xEB, 0xFD,
	}
	copy(sector, code)
	copy(sector[len(code):], "\r\n"+msg+"\r\n\x00")
	sector[510], sector[511] = 0x55, 0xAA
	return sector
}

func (h *harness) localDiskStage(b *bootimus, mac string, uefi bool) {
	t := h.t
	t.Helper()
	b.t = t
	name := "bios-localdisk"
	bootfile := loaderBIOS
	if uefi {
		name = "uefi-localdisk"
		bootfile = loaderUEFI
	}

	theme := b.mustAPI(http.MethodGet, "/api/theme", nil)
	previous, _ := theme["data"].(map[string]any)
	b.mustAPI(http.MethodPut, "/api/theme", map[string]any{"title": serialMenuTitle, "menu_timeout": 3, "default_menu_item": "local"})
	t.Cleanup(func() {
		if previous != nil {
			b.mustAPI(http.MethodPut, "/api/theme", map[string]any{"title": previous["title"], "menu_timeout": previous["menu_timeout"], "default_menu_item": previous["default_menu_item"]})
		}
	})

	var disk string
	if uefi {
		disk = h.fatDirWith(name+"-disk", filepath.Join(h.cfg.repoRoot, "bootloaders", "default", loaderUEFI))
	} else {
		disk = filepath.Join(h.cfg.runDir, name, "disk.img")
		if err := os.MkdirAll(filepath.Dir(disk), 0o755); err != nil {
			t.Fatal(err)
		}
		img := make([]byte, 1<<20)
		copy(img, biosBootSectorPrinting(serialLocalDiskMBR))
		if err := os.WriteFile(disk, img, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	staging := h.stagingDir(b)
	v := h.startVM(vmOptions{name: name, mac: mac, uefi: uefi, bootfile: bootfile, tftpDir: staging, localDisk: disk})
	if uefi && h.cfg.uefiMode != "direct" {
		h.waitFor(h.scale(90*time.Second), "firmware PXE to reach iPXE or give up", func() (bool, string) {
			return v.serialContains(serialBanner) || v.serialContains(serialNoBootUEFI) || v.serialContains(serialPXEFailed) || v.exited(), v.serialTail()
		})
	}
	if uefi && !v.serialContains(serialBanner) {
		if h.cfg.uefiMode == "pxe" {
			t.Fatalf("firmware PXE did not reach iPXE and %s=pxe forbids the direct launch fallback:\n%s", envUEFIMode, v.serialTail())
		}
		v.stop()
		t.Logf("firmware PXE did not reach iPXE (%s); launching %s from a FAT volume instead", strings.TrimSpace(truncate(v.serialTail(), 200)), loaderUEFI)
		fat := h.fatDirWith(name, filepath.Join(h.cfg.repoRoot, "bootloaders", "default", loaderUEFI))
		v = h.startVM(vmOptions{name: name + "-direct", mac: mac, uefi: true, tftpDir: staging, fatDir: fat, localDisk: disk})
	}

	h.waitFor(h.scale(90*time.Second), "iPXE banner on the "+name+" serial console", func() (bool, string) {
		return v.serialContains(serialBanner), v.serialTail()
	})
	h.waitFor(h.scale(90*time.Second), name+" menu title on the serial console", func() (bool, string) {
		return v.serialContains(serialMenuTitle), v.serialTail()
	})
	h.waitFor(h.scale(30*time.Second), name+" menu timed out into the local disk entry", func() (bool, string) {
		return v.serialContains(serialLocalBoot), v.serialTail()
	})
	if !uefi {
		h.waitFor(h.scale(60*time.Second), name+" boot sector started from the local disk", func() (bool, string) {
			return v.serialContains(serialLocalDiskMBR), v.serialTail()
		})
		v.stop()
		return
	}
	h.waitFor(h.scale(30*time.Second), name+" iPXE handed control back to the firmware", func() (bool, string) {
		return v.serialContains(serialFirmwareReturn), v.serialTail()
	})
	h.waitFor(h.scale(5*time.Minute), name+" firmware moved on to the local disk (second iPXE banner)", func() (bool, string) {
		if v.serialContains(serialNoBootUEFI) {
			t.Fatalf("firmware found nothing to boot after iPXE exited:\n%s", v.serialTail())
		}
		return v.serialCount(serialBanner) >= 2, v.serialTail()
	})
	v.stop()
}
