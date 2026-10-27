package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMenuLocalBootSetsMarkerAndTriesBIOSDisk(t *testing.T) {
	out := testMenuBuilder(nil).Build()
	local := out[strings.Index(out, ":local\n"):]
	local = local[:strings.Index(local, ":shell")]
	for _, want := range []string{"set localboot 1", "iseq ${platform} pcbios && sanboot --no-describe --drive 0x80 ||", "exit\n"} {
		if !strings.Contains(local, want) {
			t.Errorf("local boot section lacks %q:\n%s", want, local)
		}
	}
	if strings.Contains(local, "exit 1") {
		t.Errorf("a chained menu must return to its caller with a success status:\n%s", local)
	}
}

func TestAutoexecScriptsReturnToFirmwareAfterLocalBoot(t *testing.T) {
	server, _ := newBootloaderSelectionTestServer(t)
	server.config.ServerAddr = "192.0.2.10"
	server.config.HTTPPort = 8080

	capture := &tftpCapture{}
	if err := server.serveTFTPFile("autoexec.ipxe", capture); err != nil {
		t.Fatalf("serveTFTPFile: %v", err)
	}
	rec := httptest.NewRecorder()
	server.handleAutoexec(rec, httptest.NewRequest(http.MethodGet, "/autoexec.ipxe?mac=52:54:00:00:00:01", nil))

	for name, script := range map[string]string{"tftp": capture.String(), "http": rec.Body.String()} {
		check := strings.Index(script, ipxeLocalBootCheck)
		chain := strings.Index(script, "chain http://192.0.2.10:8080/inventory")
		if chain < 0 || check < chain {
			t.Errorf("%s autoexec must test the local boot marker after the menu chain:\n%s", name, script)
		}
		if !strings.Contains(script, ":localboot\nisset ${bootimus-embed} && exit ||\n") {
			t.Errorf("%s autoexec must return quietly to the embedded script when chained from it:\n%s", name, script)
		}
		if !strings.HasSuffix(strings.TrimSpace(script), "exit 1") {
			t.Errorf("%s autoexec must end by returning a failure status to the firmware:\n%s", name, script)
		}
	}
}
