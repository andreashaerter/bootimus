package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	envGate          = "BOOTIMUS_E2E"
	envBinary        = "BOOTIMUS_E2E_BINARY"
	envRunDir        = "BOOTIMUS_E2E_RUN_DIR"
	envCacheDir      = "BOOTIMUS_E2E_CACHE"
	envHTTPPort      = "BOOTIMUS_E2E_HTTP_PORT"
	envAdminPort     = "BOOTIMUS_E2E_ADMIN_PORT"
	envNetwork       = "BOOTIMUS_E2E_NETWORK"
	envRequireBridge = "BOOTIMUS_E2E_REQUIRE_BRIDGE"
	envUEFIMode      = "BOOTIMUS_E2E_UEFI"

	tftpPort = 69

	macBIOS   = "52:54:00:e2:e0:01"
	macUEFI   = "52:54:00:e2:e0:02"
	macAlpine = "52:54:00:e2:e0:03"
)

type config struct {
	repoRoot      string
	binary        string
	runDir        string
	cacheDir      string
	httpPort      int
	adminPort     int
	network       string
	requireBridge bool
	uefiMode      string
	kvm           bool
}

type harness struct {
	t   *testing.T
	cfg config
	net *network
}

func loadConfig(t *testing.T) config {
	t.Helper()
	if os.Getenv(envGate) != "1" {
		t.Skipf("set %s=1 to run the QEMU end-to-end suite (make test-e2e)", envGate)
	}
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config{
		repoRoot:      repoRoot,
		binary:        os.Getenv(envBinary),
		runDir:        os.Getenv(envRunDir),
		cacheDir:      os.Getenv(envCacheDir),
		httpPort:      envInt(t, envHTTPPort, 8080),
		adminPort:     envInt(t, envAdminPort, 18081),
		network:       os.Getenv(envNetwork),
		requireBridge: os.Getenv(envRequireBridge) == "1",
		uefiMode:      os.Getenv(envUEFIMode),
	}
	switch cfg.network {
	case "", "auto":
		cfg.network = "auto"
	case networkBridge, networkSlirp:
	default:
		t.Fatalf("%s must be auto, %s or %s", envNetwork, networkBridge, networkSlirp)
	}
	if cfg.uefiMode == "" {
		cfg.uefiMode = "auto"
	}
	if cfg.runDir == "" {
		cfg.runDir = filepath.Join(repoRoot, ".e2e", "run")
	}
	if cfg.cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		cfg.cacheDir = filepath.Join(base, "bootimus-e2e")
	}
	if err := os.RemoveAll(cfg.runDir); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{cfg.runDir, cfg.cacheDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0); err == nil {
		f.Close()
		cfg.kvm = true
	}
	t.Logf("run directory %s (kvm=%v)", cfg.runDir, cfg.kvm)
	return cfg
}

func envInt(t *testing.T, name string, def int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%s=%q is not a number", name, raw)
	}
	return n
}

func (h *harness) scale(d time.Duration) time.Duration {
	if h.cfg.kvm {
		return d
	}
	return d * 4
}

func (h *harness) sub(t *testing.T) *harness {
	return &harness{t: t, cfg: h.cfg, net: h.net}
}

func (h *harness) buildBinary() string {
	t := h.t
	t.Helper()
	if h.cfg.binary != "" {
		if _, err := os.Stat(h.cfg.binary); err != nil {
			t.Fatalf("%s: %v", envBinary, err)
		}
		return h.cfg.binary
	}
	out := filepath.Join(h.cfg.runDir, "bootimus")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = h.cfg.repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	t.Log("building bootimus")
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	h.cfg.binary = out
	return out
}

func portFree(addr string) bool {
	l, err := net.Listen("tcp4", addr)
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

type bootimus struct {
	t          *testing.T
	name       string
	cmd        *exec.Cmd
	logPath    string
	dataDir    string
	serverAddr string
	httpPort   int
	adminPort  int
	adminURL   string
	bootURL    string
	guestURL   string
	token      string
	tftpByQEMU bool
	stagingDir string
	done       chan error
}

func (h *harness) startBootimus() *bootimus {
	t := h.t
	t.Helper()
	binary := h.buildBinary()
	for _, p := range []string{fmt.Sprintf(":%d", h.cfg.httpPort), fmt.Sprintf(":%d", h.cfg.adminPort)} {
		if !portFree(p) {
			t.Fatalf("TCP port %s is already in use; set %s or %s to free ports", p, envHTTPPort, envAdminPort)
		}
	}
	dir := filepath.Join(h.cfg.runDir, "bootimus")
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	b := &bootimus{
		t:          t,
		name:       "bootimus",
		logPath:    filepath.Join(dir, "bootimus.log"),
		dataDir:    dataDir,
		serverAddr: h.net.hostAddr,
		httpPort:   h.cfg.httpPort,
		adminPort:  h.cfg.adminPort,
		guestURL:   fmt.Sprintf("http://%s:%d", h.net.hostAddr, h.cfg.httpPort),
		done:       make(chan error, 1),
	}
	args := []string{"serve",
		"--data-dir", dataDir,
		"--server-addr", b.serverAddr,
		"--http-port", strconv.Itoa(b.httpPort),
		"--admin-port", strconv.Itoa(b.adminPort),
		"--nbd-enabled=false",
	}
	if h.net.mode == networkBridge {
		b.adminURL = fmt.Sprintf("http://%s:%d", h.net.hostAddr, b.adminPort)
		args = append(args, "--tftp-port", strconv.Itoa(tftpPort), "--proxy-dhcp", "--bind-interface", h.net.bridge)
	} else {
		b.adminURL = fmt.Sprintf("http://127.0.0.1:%d", b.adminPort)
		b.tftpByQEMU = true
		args = append(args, "--tftp-port", strconv.Itoa(freeUDPPort(t)))
	}
	b.bootURL = strings.Replace(b.adminURL, strconv.Itoa(b.adminPort), strconv.Itoa(b.httpPort), 1)

	logFile, err := os.Create(b.logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "BOOTIMUS_DISABLE_REMOTE_PROFILES=true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start bootimus: %v", err)
	}
	b.cmd = cmd
	go func() {
		b.done <- cmd.Wait()
		logFile.Close()
	}()
	t.Cleanup(b.stop)
	t.Logf("bootimus: pid %d (%s)", cmd.Process.Pid, strings.Join(args, " "))

	h.waitFor(h.scale(60*time.Second), "bootimus admin interface", func() (bool, string) {
		select {
		case err := <-b.done:
			b.done <- err
			t.Fatalf("bootimus exited early: %v\n%s", err, tail(b.logPath, 30))
		default:
		}
		resp, err := http.Get(b.adminURL + "/health")
		if err != nil {
			return false, err.Error()
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK, resp.Status
	})
	if h.net.mode == networkBridge {
		h.waitFor(h.scale(15*time.Second), "bootimus TFTP and proxyDHCP listeners", func() (bool, string) {
			if line := grepLine(b.logPath, "TFTP server error"); line != "" {
				t.Fatalf("bootimus could not bind UDP/%d; grant the binary cap_net_bind_service (sudo setcap cap_net_bind_service=+ep %s): %s", tftpPort, binary, line)
			}
			if line := grepLine(b.logPath, "proxyDHCP: failed"); line != "" {
				t.Fatalf("bootimus proxyDHCP did not start: %s", line)
			}
			return b.logContains("proxyDHCP: listening on UDP/67"), tail(b.logPath, 5)
		})
	}
	b.login()
	return b
}

func (b *bootimus) stop() {
	if b.cmd == nil || b.cmd.Process == nil {
		return
	}
	select {
	case err := <-b.done:
		b.done <- err
		return
	default:
	}
	b.cmd.Process.Signal(syscall.SIGTERM)
	var err error
	select {
	case err = <-b.done:
	case <-time.After(15 * time.Second):
		b.cmd.Process.Kill()
		err = <-b.done
	}
	b.done <- err
}

var passwordLine = regexp.MustCompile(`Password:\s+(\S+)`)

func (b *bootimus) login() {
	t := b.t
	t.Helper()
	var password string
	deadline := time.Now().Add(30 * time.Second)
	for password == "" {
		data, _ := os.ReadFile(b.logPath)
		if m := passwordLine.FindSubmatch(data); m != nil {
			password = string(m[1])
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("generated admin password not found in %s", b.logPath)
		}
		time.Sleep(200 * time.Millisecond)
	}
	resp := b.api(http.MethodPost, "/api/login", map[string]string{"username": "admin", "password": password})
	data, _ := resp["data"].(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("login failed: %v", resp)
	}
	b.token = token
}

func (b *bootimus) do(method, path string, body io.Reader, contentType string) (*http.Response, []byte) {
	t := b.t
	t.Helper()
	req, err := http.NewRequest(method, b.adminURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (b *bootimus) api(method, path string, body any) map[string]any {
	t := b.t
	t.Helper()
	var reader io.Reader
	contentType := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
		contentType = "application/json"
	}
	resp, data := b.do(method, path, reader, contentType)
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s %s: HTTP %d, not JSON: %s", method, path, resp.StatusCode, truncate(string(data), 300))
	}
	out["_status"] = resp.StatusCode
	return out
}

func (b *bootimus) mustAPI(method, path string, body any) map[string]any {
	t := b.t
	t.Helper()
	out := b.api(method, path, body)
	if ok, _ := out["success"].(bool); !ok {
		t.Fatalf("%s %s failed: %v", method, path, out)
	}
	return out
}

func (b *bootimus) upload(path, field, filename string, content []byte, fields map[string]string) map[string]any {
	t := b.t
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	w.Close()
	resp, data := b.do(http.MethodPost, path, &buf, w.FormDataContentType())
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("POST %s: HTTP %d, not JSON: %s", path, resp.StatusCode, truncate(string(data), 300))
	}
	if ok, _ := out["success"].(bool); !ok {
		t.Fatalf("POST %s failed: %v", path, out)
	}
	return out
}

func (b *bootimus) metric(name string) float64 {
	t := b.t
	t.Helper()
	resp, err := http.Get(b.adminURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, name+" ") {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, name)), 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}

func (b *bootimus) logContains(substr string) bool {
	return fileContains(b.logPath, substr)
}

func (b *bootimus) clientKnown(mac string) (bool, string) {
	out := b.api(http.MethodGet, "/api/clients?mac="+mac, nil)
	if ok, _ := out["success"].(bool); !ok {
		return false, fmt.Sprint(out["error"])
	}
	switch data := out["data"].(type) {
	case map[string]any:
		return strings.EqualFold(fmt.Sprint(data["mac_address"]), mac), fmt.Sprint(data["mac_address"])
	case []any:
		for _, c := range data {
			client, _ := c.(map[string]any)
			if strings.EqualFold(fmt.Sprint(client["mac_address"]), mac) {
				return true, mac
			}
		}
		return false, fmt.Sprintf("%d clients, none with %s", len(data), mac)
	}
	return false, "no client"
}

func (b *bootimus) inventory(mac string) map[string]any {
	out := b.api(http.MethodGet, "/api/clients/inventory?mac="+mac, nil)
	if ok, _ := out["success"].(bool); !ok {
		return nil
	}
	data, _ := out["data"].(map[string]any)
	return data
}

func (b *bootimus) proxyDHCPReplied(mac string) (bool, string) {
	out := b.api(http.MethodGet, "/api/proxydhcp/events", nil)
	events, _ := out["events"].([]any)
	for _, e := range events {
		ev, _ := e.(map[string]any)
		if strings.EqualFold(fmt.Sprint(ev["mac"]), mac) && fmt.Sprint(ev["outcome"]) == "replied" {
			return true, fmt.Sprint(ev["decision"])
		}
	}
	return false, fmt.Sprintf("%d proxyDHCP events, none replied to %s", len(events), mac)
}

func (h *harness) waitFor(timeout time.Duration, what string, cond func() (bool, string)) {
	t := h.t
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		ok, detail := cond()
		if ok {
			t.Logf("ok: %s", what)
			return
		}
		last = detail
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s (last: %s); logs are under %s", timeout, what, last, h.cfg.runDir)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte(substr))
}

func grepLine(path, substr string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

func tail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	parts := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
