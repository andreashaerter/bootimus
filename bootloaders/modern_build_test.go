package bootloaders

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestModernBuilderRejectsUnsafeInputs(test *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		test.Skip("bash is unavailable")
	}
	if _, err := exec.LookPath("grep"); err != nil {
		test.Skip("grep is unavailable")
	}
	directory := test.TempDir()
	privateKey := filepath.Join(directory, "private.pem")
	if err := os.WriteFile(privateKey, []byte("-----BEGIN PRIVATE KEY-----\nnot-a-certificate\n"), 0600); err != nil {
		test.Fatal(err)
	}
	bundle := filepath.Join(directory, "bundle.pem")
	if err := os.WriteFile(bundle, []byte("-----BEGIN CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\n"), 0600); err != nil {
		test.Fatal(err)
	}
	for _, scenario := range []struct {
		name      string
		arguments []string
		message   string
	}{
		{"usage", nil, "Usage:"},
		{"existing directory", []string{directory, privateKey}, "already exists"},
		{"private key", []string{filepath.Join(directory, "key-output"), privateKey}, "never private keys"},
		{"certificate bundle", []string{filepath.Join(directory, "bundle-output"), bundle}, "one PEM certificate"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			arguments := append([]string{"../scripts/build-modern-bootloaders.sh"}, scenario.arguments...)
			command := exec.Command("bash", arguments...)
			command.Env = append(os.Environ(), "CONTAINER_ENGINE=must-not-run", "TMPDIR="+directory)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), scenario.message) {
				test.Fatalf("unexpected rejection: %v: %s", err, output)
			}
		})
	}
}
