package smb

import (
	"net"
	"os"
	"strings"
	"testing"
)

func TestBindTarget(t *testing.T) {
	local := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("192.168.1.10")}
	cases := []struct {
		name, iface, addr, want string
	}{
		{"bind interface wins", "eth0", "192.168.1.10", "eth0"},
		{"local server address", "", "192.168.1.10", "192.168.1.10"},
		{"address not on this host", "", "10.96.0.15", ""},
		{"hostname", "", "bootimus.lan", ""},
		{"no address", "", "", ""},
	}
	for _, c := range cases {
		if got := bindTarget(c.iface, c.addr, local); got != c.want {
			t.Errorf("%s: bindTarget(%q, %q) = %q, want %q", c.name, c.iface, c.addr, got, c.want)
		}
	}
}

func TestWriteConfigBinding(t *testing.T) {
	for _, c := range []struct {
		interfaces string
		wantBind   bool
	}{
		{"192.168.1.10", true},
		{"", false},
	} {
		m := &Manager{dataDir: t.TempDir(), interfaces: c.interfaces, port: 445, shares: map[string]string{}}
		if err := os.MkdirAll(m.smbDir(), 0755); err != nil {
			t.Fatal(err)
		}
		if err := m.writeConfig(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(m.configPath())
		if err != nil {
			t.Fatal(err)
		}
		conf := string(data)
		hasBind := strings.Contains(conf, "bind interfaces only = yes")
		if hasBind != c.wantBind {
			t.Errorf("interfaces %q: bind interfaces only present = %v, want %v\n%s", c.interfaces, hasBind, c.wantBind, conf)
		}
		if c.wantBind && !strings.Contains(conf, "interfaces = "+c.interfaces+"\n") {
			t.Errorf("interfaces %q: missing interfaces line\n%s", c.interfaces, conf)
		}
		if !strings.Contains(conf, "log file = ") || !strings.Contains(conf, "\nsmb ports = 445\n") {
			t.Errorf("interfaces %q: surrounding config lines malformed\n%s", c.interfaces, conf)
		}
	}
}
