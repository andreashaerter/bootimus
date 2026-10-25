package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	networkBridge = "bridge"
	networkSlirp  = "slirp"

	bridgeHostAddr = "10.77.0.1"
	bridgeDHCPAddr = "10.77.0.2"
	bridgePoolFrom = "10.77.0.50"
	bridgePrefix   = "24"

	slirpHostAddr = "10.0.2.2"
	slirpSubnet   = "10.0.2.0/24"
)

type network struct {
	mode     string
	hostAddr string
	id       string
	bridge   string
	tap      string
	vethHost string
	vethNS   string
	netns    string
	priv     []string
	dhcpd    *exec.Cmd
	dhcpdBin string
	logPath  string
}

func privilegePrefix() ([]string, bool) {
	if os.Geteuid() == 0 {
		return nil, true
	}
	if exec.Command("sudo", "-n", "true").Run() == nil {
		return []string{"sudo", "-n"}, true
	}
	return nil, false
}

func (n *network) command(args ...string) *exec.Cmd {
	full := append(append([]string{}, n.priv...), args...)
	return exec.Command(full[0], full[1:]...)
}

func (n *network) run(args ...string) error {
	out, err := n.command(args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (h *harness) setupNetwork() *network {
	t := h.t
	t.Helper()
	mode := h.cfg.network
	priv, privileged := privilegePrefix()
	_, haveIP := exec.LookPath("ip")
	switch {
	case mode == networkSlirp:
		if h.cfg.requireBridge {
			t.Fatalf("%s=1 conflicts with %s=%s", envRequireBridge, envNetwork, networkSlirp)
		}
	case !privileged || haveIP != nil:
		if h.cfg.requireBridge || mode == networkBridge {
			t.Fatalf("bridge networking needs root or passwordless sudo and iproute2 (sudo -n: %v, ip: %v)", privileged, haveIP == nil)
		}
		t.Logf("no root or passwordless sudo: using QEMU user-mode networking, so Bootimus TFTP and proxyDHCP are not exercised")
		mode = networkSlirp
	default:
		mode = networkBridge
	}
	if mode == networkSlirp {
		return &network{mode: networkSlirp, hostAddr: slirpHostAddr}
	}

	id := fmt.Sprintf("%04x", os.Getpid()&0xffff)
	n := &network{
		mode:     networkBridge,
		hostAddr: bridgeHostAddr,
		id:       id,
		bridge:   "bte2e" + id,
		tap:      "tpe2e" + id,
		vethHost: "vhe2e" + id,
		vethNS:   "vne2e" + id,
		netns:    "bootimus-e2e-" + id,
		priv:     priv,
		logPath:  filepath.Join(h.cfg.runDir, "network", "dhcpd.log"),
	}
	if err := os.MkdirAll(filepath.Dir(n.logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	n.cleanupStale()
	t.Cleanup(n.teardown)
	steps := [][]string{
		{"ip", "link", "add", n.bridge, "type", "bridge"},
		{"ip", "addr", "add", bridgeHostAddr + "/" + bridgePrefix, "dev", n.bridge},
		{"ip", "link", "set", n.bridge, "up"},
		{"ip", "tuntap", "add", "dev", n.tap, "mode", "tap", "user", me.Username},
		{"ip", "link", "set", n.tap, "master", n.bridge},
		{"ip", "link", "set", n.tap, "up"},
		{"ip", "netns", "add", n.netns},
		{"ip", "link", "add", n.vethHost, "type", "veth", "peer", "name", n.vethNS},
		{"ip", "link", "set", n.vethHost, "master", n.bridge},
		{"ip", "link", "set", n.vethHost, "up"},
		{"ip", "link", "set", n.vethNS, "netns", n.netns},
		{"ip", "netns", "exec", n.netns, "ip", "addr", "add", bridgeDHCPAddr + "/" + bridgePrefix, "dev", n.vethNS},
		{"ip", "netns", "exec", n.netns, "ip", "link", "set", n.vethNS, "up"},
		{"ip", "netns", "exec", n.netns, "ip", "link", "set", "lo", "up"},
	}
	for _, step := range steps {
		if err := n.run(step...); err != nil {
			t.Fatalf("bridge setup failed: %v", err)
		}
	}
	h.startDHCP(n)
	t.Logf("bridge network %s (%s) with DHCP from %s in namespace %s", n.bridge, bridgeHostAddr, bridgeDHCPAddr, n.netns)
	return n
}

func (h *harness) startDHCP(n *network) {
	t := h.t
	t.Helper()
	n.dhcpdBin = filepath.Join(h.cfg.runDir, "dhcpd")
	build := exec.Command("go", "build", "-o", n.dhcpdBin, "./dhcpd")
	build.Dir = filepath.Join(h.cfg.repoRoot, "e2e")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build dhcpd helper: %v: %s", err, out)
	}
	logFile, err := os.Create(n.logPath)
	if err != nil {
		t.Fatal(err)
	}
	n.dhcpd = n.command("ip", "netns", "exec", n.netns, n.dhcpdBin,
		"-iface", n.vethNS, "-server", bridgeDHCPAddr, "-start", bridgePoolFrom, "-count", "10")
	n.dhcpd.Stdout = logFile
	n.dhcpd.Stderr = logFile
	if err := n.dhcpd.Start(); err != nil {
		t.Fatalf("start dhcpd: %v", err)
	}
	go func() {
		n.dhcpd.Wait()
		logFile.Close()
	}()
	h.waitFor(15*time.Second, "DHCP helper listening in the namespace", func() (bool, string) {
		return fileContains(n.logPath, "dhcpd: listening"), tail(n.logPath, 5)
	})
}

func (n *network) cleanupStale() {
	out, err := exec.Command("ip", "-o", "link", "show").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			name := strings.TrimSuffix(fields[1], ":")
			if i := strings.Index(name, "@"); i >= 0 {
				name = name[:i]
			}
			for _, prefix := range []string{"bte2e", "tpe2e", "vhe2e"} {
				if strings.HasPrefix(name, prefix) {
					n.run("ip", "link", "del", name)
				}
			}
		}
	}
	out, err = exec.Command("ip", "netns", "list").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			name := strings.Fields(line)
			if len(name) > 0 && strings.HasPrefix(name[0], "bootimus-e2e-") {
				n.run("ip", "netns", "del", name[0])
			}
		}
	}
}

func (n *network) teardown() {
	if n.dhcpd != nil && n.dhcpd.Process != nil {
		n.dhcpd.Process.Signal(syscall.SIGTERM)
		n.run("pkill", "-f", n.dhcpdBin)
	}
	n.run("ip", "netns", "del", n.netns)
	n.run("ip", "link", "del", n.vethHost)
	n.run("ip", "link", "del", n.tap)
	n.run("ip", "link", "del", n.bridge)
}

func (n *network) qemuNetdev(tftpDir, bootfile string) string {
	if n.mode == networkBridge {
		return "tap,id=n0,ifname=" + n.tap + ",script=no,downscript=no"
	}
	netdev := "user,id=n0,net=" + slirpSubnet + ",tftp-server-name=" + slirpHostAddr
	if tftpDir != "" {
		netdev += ",tftp=" + tftpDir
	}
	if bootfile != "" {
		netdev += ",bootfile=" + bootfile
	}
	return netdev
}
