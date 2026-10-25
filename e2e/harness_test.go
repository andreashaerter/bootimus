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
	envGate        = "BOOTIMUS_E2E"
	envBinary      = "BOOTIMUS_E2E_BINARY"
	envRunDir      = "BOOTIMUS_E2E_RUN_DIR"
	envCacheDir    = "BOOTIMUS_E2E_CACHE"
	envHTTPPort    = "BOOTIMUS_E2E_HTTP_PORT"
	envAdminPort   = "BOOTIMUS_E2E_ADMIN_PORT"
	envTFTPPort    = "BOOTIMUS_E2E_TFTP_PORT"
	envRequireTFTP = "BOOTIMUS_E2E_REQUIRE_TFTP"
	envProxyDHCP   = "BOOTIMUS_E2E_PROXYDHCP"
	envUEFIMode    = "BOOTIMUS_E2E_UEFI"

	guestServerAddr = "10.0.2.2"
	guestSubnet     = "10.0.2.0/24"

	macBIOS      = "52:54:00:e2:e0:01"
	macUEFI      = "52:54:00:e2:e0:02"
	macAlpine    = "52:54:00:e2:e0:03"
	macProxyDHCP = "52:54:00:e2:e0:04"
)

type config struct {
	repoRoot    string
	binary      string
	runDir      string
	cacheDir    string
	httpPort    int
	adminPort   int
	tftpPort    int
	requireTFTP bool
	proxyDHCP   bool
	uefiMode    string
	kvm         bool
}

type harness struct {
	t   *testing.T
	cfg config
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
		repoRoot:    repoRoot,
		binary:      os.Getenv(envBinary),
		runDir:      os.Getenv(envRunDir),
		cacheDir:    os.Getenv(envCacheDir),
		httpPort:    envInt(t, envHTTPPort, 8080),
		adminPort:   envInt(t, envAdminPort, 18081),
		tftpPort:    envInt(t, envTFTPPort, 69),
		requireTFTP: os.Getenv(envRequireTFTP) == "1",
		proxyDHCP:   os.Getenv(envProxyDHCP) == "1",
		uefiMode:    os.Getenv(envUEFIMode),
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

func portFree(network, addr string) bool {
	switch network {
	case "udp":
		c, err := net.ListenPacket("udp4", addr)
		if err != nil {
			return false
		}
		c.Close()
	default:
		l, err := net.Listen("tcp4", addr)
		if err != nil {
			return false
		}
		l.Close()
	}
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
	tftpPort   int
	adminURL   string
	token      string
	tftpByQEMU bool
	stagingDir string
	done       chan error
}

type serverOptions struct {
	name       string
	serverAddr string
	adminHost  string
	httpPort   int
	adminPort  int
	tftpPort   int
	extraArgs  []string
}

func (h *harness) startBootimus(o serverOptions) *bootimus {
	t := h.t
	t.Helper()
	binary := h.buildBinary()
	for _, p := range []string{fmt.Sprintf(":%d", o.httpPort), fmt.Sprintf(":%d", o.adminPort)} {
		if !portFree("tcp", p) {
			t.Fatalf("TCP port %s is already in use; set %s or %s to free ports", p, envHTTPPort, envAdminPort)
		}
	}
	b, err := h.launch(binary, o, o.tftpPort, false)
	if err == nil {
		return b
	}
	if o.tftpPort >= 1024 || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("%s: %v", o.name, err)
	}
	if h.cfg.requireTFTP {
		t.Fatalf("%s: cannot bind UDP %d for TFTP and %s=1; grant the binary cap_net_bind_service (sudo setcap cap_net_bind_service=+ep %s): %v", o.name, o.tftpPort, envRequireTFTP, binary, err)
	}
	fallback := freeUDPPort(t)
	t.Logf("%s: UDP %d is privileged here; Bootimus TFTP moves to %d and QEMU's built-in TFTP serves the loader, so the TFTP stage is not exercised", o.name, o.tftpPort, fallback)
	b, err = h.launch(binary, o, fallback, true)
	if err != nil {
		t.Fatalf("%s: %v", o.name, err)
	}
	return b
}

func (h *harness) launch(binary string, o serverOptions, tftpPort int, tftpByQEMU bool) (*bootimus, error) {
	t := h.t
	t.Helper()
	dir := filepath.Join(h.cfg.runDir, o.name)
	dataDir := filepath.Join(dir, "data")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	b := &bootimus{
		t:          t,
		name:       o.name,
		logPath:    filepath.Join(dir, "bootimus.log"),
		dataDir:    dataDir,
		serverAddr: o.serverAddr,
		httpPort:   o.httpPort,
		adminPort:  o.adminPort,
		tftpPort:   tftpPort,
		tftpByQEMU: tftpByQEMU,
		done:       make(chan error, 1),
	}
	adminHost := o.adminHost
	if adminHost == "" {
		adminHost = "127.0.0.1"
	}
	b.adminURL = fmt.Sprintf("http://%s:%d", adminHost, o.adminPort)

	args := []string{"serve",
		"--data-dir", dataDir,
		"--server-addr", o.serverAddr,
		"--http-port", strconv.Itoa(o.httpPort),
		"--admin-port", strconv.Itoa(o.adminPort),
		"--tftp-port", strconv.Itoa(tftpPort),
		"--nbd-enabled=false",
	}
	args = append(args, o.extraArgs...)
	logFile, err := os.Create(b.logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "BOOTIMUS_DISABLE_REMOTE_PROFILES=true")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bootimus: %w", err)
	}
	b.cmd = cmd
	go func() {
		b.done <- cmd.Wait()
		logFile.Close()
	}()
	t.Cleanup(b.stop)
	t.Logf("%s: started pid %d (args %s)", o.name, cmd.Process.Pid, strings.Join(args, " "))

	deadline := time.Now().Add(h.scale(60 * time.Second))
	for {
		select {
		case err := <-b.done:
			b.done <- err
			return nil, fmt.Errorf("process exited early: %v\n%s", err, tail(b.logPath, 30))
		default:
		}
		resp, err := http.Get(b.adminURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("admin interface did not become healthy; log tail:\n%s", tail(b.logPath, 30))
		}
		time.Sleep(300 * time.Millisecond)
	}
	settle := time.Now().Add(3 * time.Second)
	for time.Now().Before(settle) {
		if b.logContains("TFTP server error") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if b.logContains("TFTP server error") {
		b.stop()
		return nil, fmt.Errorf("TFTP listener failed: %s", grepLine(b.logPath, "TFTP server error"))
	}
	b.login()
	return b, nil
}

func grepLine(path, substr string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
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
			t.Fatalf("%s: generated admin password not found in %s", b.name, b.logPath)
		}
		time.Sleep(200 * time.Millisecond)
	}
	resp := b.api(http.MethodPost, "/api/login", map[string]string{"username": "admin", "password": password})
	data, _ := resp["data"].(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("%s: login failed: %v", b.name, resp)
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
