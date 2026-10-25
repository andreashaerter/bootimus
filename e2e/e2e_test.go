package e2e

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestE2E(t *testing.T) {
	cfg := loadConfig(t)
	h := &harness{t: t, cfg: cfg}
	qemuBinary(t)

	b := h.startBootimus(serverOptions{
		name:       "primary",
		serverAddr: guestServerAddr,
		httpPort:   cfg.httpPort,
		adminPort:  cfg.adminPort,
		tftpPort:   cfg.tftpPort,
	})

	t.Run("BIOSMenu", func(t *testing.T) {
		sub := &harness{t: t, cfg: cfg}
		sub.menuStage(b, macBIOS, false)
	})
	t.Run("UEFIMenu", func(t *testing.T) {
		sub := &harness{t: t, cfg: cfg}
		sub.menuStage(b, macUEFI, true)
	})
	t.Run("AlpineNextBoot", func(t *testing.T) {
		sub := &harness{t: t, cfg: cfg}
		sub.alpineStage(b)
	})
	t.Run("ProxyDHCP", func(t *testing.T) {
		sub := &harness{t: t, cfg: cfg}
		sub.proxyDHCPStage(b)
	})
}

func (h *harness) menuStage(b *bootimus, mac string, uefi bool) {
	t := h.t
	t.Helper()
	b.t = t
	name := "bios"
	bootfile := loaderBIOS
	if uefi {
		name = "uefi"
		bootfile = loaderUEFI
	}
	staging := h.stagingDir(b)
	v := h.startVM(vmOptions{name: name, mac: mac, uefi: uefi, bootfile: bootfile, tftpDir: staging})

	if uefi && h.cfg.uefiMode != "direct" {
		h.waitFor(h.scale(60*time.Second), "firmware to attempt PXE or report no boot option", func() (bool, string) {
			return v.serialContains(serialBanner) || v.serialContains(serialNoBootUEFI) || v.serialContains(serialPXEFailed) || v.exited(), v.serialTail()
		})
	}
	if uefi && !v.serialContains(serialBanner) {
		if h.cfg.uefiMode == "pxe" {
			t.Fatalf("firmware did not PXE boot and %s=pxe forbids the direct launch fallback:\n%s", envUEFIMode, v.serialTail())
		}
		v.stop()
		t.Logf("firmware PXE did not reach iPXE (%s); launching %s directly from a FAT volume instead, so the firmware's own TFTP download is not exercised", strings.TrimSpace(truncate(v.serialTail(), 200)), loaderUEFI)
		fat := h.fatDirWith(name, h.cfg.repoRoot+"/bootloaders/default/"+loaderUEFI)
		v = h.startVM(vmOptions{name: name + "-direct", mac: mac, uefi: true, tftpDir: staging, fatDir: fat})
	}

	h.waitFor(h.scale(90*time.Second), "iPXE banner on the "+name+" serial console", func() (bool, string) {
		return v.serialContains(serialBanner), v.serialTail()
	})
	h.waitFor(h.scale(90*time.Second), name+" client discovered with inventory", func() (bool, string) {
		known, detail := b.clientKnown(mac)
		if !known {
			return false, "client: " + detail
		}
		inv := b.inventory(mac)
		if inv == nil {
			return false, "no inventory yet"
		}
		manufacturer := fmt.Sprint(inv["manufacturer"])
		return strings.Contains(strings.ToUpper(manufacturer), "QEMU"), "manufacturer=" + manufacturer
	})
	h.waitFor(h.scale(30*time.Second), name+" menu request in the server log", func() (bool, string) {
		return b.logContains("Client Connected: MAC " + mac), tail(b.logPath, 5)
	})
	h.waitFor(h.scale(60*time.Second), name+" menu title on the serial console", func() (bool, string) {
		return v.serialContains(serialMenuTitle), v.serialTail()
	})
	if !b.tftpByQEMU {
		if !b.logContains("TFTP: Client requesting file: " + bootfile) {
			t.Errorf("Bootimus TFTP never served %s; log tail:\n%s", bootfile, tail(b.logPath, 20))
		}
		if !b.logContains("TFTP: Serving dynamic autoexec.ipxe") {
			t.Errorf("Bootimus TFTP never served autoexec.ipxe")
		}
	}
	v.stop()
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
