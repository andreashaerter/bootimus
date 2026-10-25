package e2e

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

const (
	bridgeName  = "br-bootimus"
	tapName     = "tap-bootimus"
	vethHost    = "veth-bootimus"
	vethNS      = "veth-bootimus-ns"
	netnsName   = "bootimus-e2e"
	bridgeAddr  = "10.77.0.1"
	dhcpAddr    = "10.77.0.2"
	dhcpRange   = "10.77.0.50,10.77.0.60,255.255.255.0,1h"
	proxyHTTP   = 8082
	proxyAdmin  = 18082
	proxyTFTP   = 69
	proxyPrefix = "24"
)

func sudoAvailable() bool {
	return exec.Command("sudo", "-n", "true").Run() == nil
}

func (h *harness) sudo(args ...string) error {
	cmd := exec.Command("sudo", append([]string{"-n"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sudo %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *harness) mustSudo(args ...string) {
	h.t.Helper()
	if err := h.sudo(args...); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) teardownBridge() {
	h.sudo("ip", "netns", "del", netnsName)
	h.sudo("ip", "link", "del", vethHost)
	h.sudo("ip", "tuntap", "del", "dev", tapName, "mode", "tap")
	h.sudo("ip", "link", "del", bridgeName)
}

func (h *harness) proxyDHCPStage(primary *bootimus) {
	t := h.t
	t.Helper()
	if !h.cfg.proxyDHCP {
		t.Skipf("set %s=1 (needs passwordless sudo, iproute2 and dnsmasq) to run the proxyDHCP stage", envProxyDHCP)
	}
	if !sudoAvailable() {
		t.Fatalf("%s=1 but passwordless sudo is not available", envProxyDHCP)
	}
	if h.cfg.tftpPort != proxyTFTP || !portFree("udp", fmt.Sprintf(":%d", proxyTFTP)) {
		t.Fatalf("the proxyDHCP stage needs the binary to bind UDP %d; grant cap_net_bind_service", proxyTFTP)
	}
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	primary.stop()

	h.teardownBridge()
	t.Cleanup(h.teardownBridge)
	h.mustSudo("ip", "link", "add", bridgeName, "type", "bridge")
	h.mustSudo("ip", "addr", "add", bridgeAddr+"/"+proxyPrefix, "dev", bridgeName)
	h.mustSudo("ip", "link", "set", bridgeName, "up")
	h.mustSudo("ip", "tuntap", "add", "dev", tapName, "mode", "tap", "user", me.Username)
	h.mustSudo("ip", "link", "set", tapName, "master", bridgeName)
	h.mustSudo("ip", "link", "set", tapName, "up")
	h.mustSudo("ip", "netns", "add", netnsName)
	h.mustSudo("ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethNS)
	h.mustSudo("ip", "link", "set", vethHost, "master", bridgeName)
	h.mustSudo("ip", "link", "set", vethHost, "up")
	h.mustSudo("ip", "link", "set", vethNS, "netns", netnsName)
	h.mustSudo("ip", "netns", "exec", netnsName, "ip", "addr", "add", dhcpAddr+"/"+proxyPrefix, "dev", vethNS)
	h.mustSudo("ip", "netns", "exec", netnsName, "ip", "link", "set", vethNS, "up")
	h.mustSudo("ip", "netns", "exec", netnsName, "ip", "link", "set", "lo", "up")

	dir := filepath.Join(h.cfg.runDir, "proxydhcp")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0o777)
	dnsmasqLog := filepath.Join(dir, "dnsmasq.log")
	dnsmasq := exec.Command("sudo", "-n", "ip", "netns", "exec", netnsName, "dnsmasq",
		"--keep-in-foreground",
		"--conf-file=/dev/null",
		"--port=0",
		"--interface="+vethNS,
		"--dhcp-range="+dhcpRange,
		"--dhcp-leasefile="+filepath.Join(dir, "dnsmasq.leases"),
		"--pid-file="+filepath.Join(dir, "dnsmasq.pid"),
		"--log-dhcp",
		"--log-facility="+dnsmasqLog,
	)
	if err := dnsmasq.Start(); err != nil {
		t.Fatalf("start dnsmasq: %v", err)
	}
	t.Cleanup(func() {
		if data, err := os.ReadFile(filepath.Join(dir, "dnsmasq.pid")); err == nil {
			h.sudo("kill", strings.TrimSpace(string(data)))
		}
		dnsmasq.Process.Kill()
		dnsmasq.Wait()
	})

	b := h.startBootimus(serverOptions{
		name:       "proxydhcp-server",
		serverAddr: bridgeAddr,
		adminHost:  bridgeAddr,
		httpPort:   proxyHTTP,
		adminPort:  proxyAdmin,
		tftpPort:   proxyTFTP,
		extraArgs:  []string{"--proxy-dhcp", "--bind-interface", bridgeName},
	})
	h.waitFor(h.scale(30*time.Second), "proxyDHCP listener", func() (bool, string) {
		return b.logContains("proxyDHCP: listening on UDP/67"), tail(b.logPath, 5)
	})

	v := h.startVM(vmOptions{
		name:   "proxydhcp",
		mac:    macProxyDHCP,
		netdev: "tap,id=n0,ifname=" + tapName + ",script=no,downscript=no",
	})
	h.waitFor(h.scale(90*time.Second), "proxyDHCP reply recorded for the client", func() (bool, string) {
		out := b.api(http.MethodGet, "/api/proxydhcp/events", nil)
		events, _ := out["events"].([]any)
		for _, e := range events {
			ev, _ := e.(map[string]any)
			if strings.EqualFold(fmt.Sprint(ev["mac"]), macProxyDHCP) && fmt.Sprint(ev["outcome"]) == "replied" {
				return true, fmt.Sprint(ev["decision"])
			}
		}
		return false, fmt.Sprintf("%d events; dnsmasq: %s", len(events), truncate(tail(dnsmasqLog, 5), 400))
	})
	h.waitFor(h.scale(90*time.Second), "iPXE banner over the bridged network", func() (bool, string) {
		return v.serialContains(serialBanner), v.serialTail()
	})
	h.waitFor(h.scale(90*time.Second), "client discovered through proxyDHCP boot", func() (bool, string) {
		known, detail := b.clientKnown(macProxyDHCP)
		return known, detail
	})
	if !b.logContains("TFTP: Client requesting file: " + loaderBIOS) {
		t.Errorf("Bootimus TFTP did not serve %s over the bridge; log tail:\n%s", loaderBIOS, tail(b.logPath, 20))
	}
	v.stop()
}
