package smb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShareNamesRemainDistinct(t *testing.T) {
	filenames := []string{
		"Windows.11.iso", "Windows_11.iso", "Windows 11.iso", "windows.11.iso",
		"group/Windows.11.iso", "group_Windows.11.iso", "Windows.11.img",
		"日本語.iso", "中文.iso", strings.Repeat("A", 80) + "-first.iso", strings.Repeat("A", 80) + "-second.iso",
	}
	manager := &Manager{dataDir: t.TempDir(), port: 445, shares: make(map[string]string)}
	seen := make(map[string]string)
	for _, filename := range filenames {
		name := ShareName(filename)
		if len(name) > 80 || name != ShareName(filename) {
			t.Fatalf("invalid or unstable name for %q: %q", filename, name)
		}
		if other, exists := seen[strings.ToLower(name)]; exists {
			t.Fatalf("%q and %q share the name %q", filename, other, name)
		}
		seen[strings.ToLower(name)] = filename
		if err := manager.AddShare(name, filepath.Join(manager.dataDir, filename, "iso")); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.ensureStateDirs(); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeConfig(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(manager.configPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, filename := range filenames {
		if !strings.Contains(string(content), "["+ShareName(filename)+"]") || !strings.Contains(string(content), filepath.Join(manager.dataDir, filename, "iso")) {
			t.Errorf("share missing from smb.conf for %q", filename)
		}
	}
}

func TestAddShareRejectsConflictingPaths(t *testing.T) {
	manager := &Manager{shares: make(map[string]string)}
	if err := manager.AddShare("windows", "/images/first"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"windows", "WINDOWS"} {
		if err := manager.AddShare(name, "/images/first"); err != nil {
			t.Fatalf("same path should be idempotent: %v", err)
		}
		if err := manager.AddShare(name, "/images/second"); err == nil {
			t.Fatal("conflicting path was accepted")
		}
	}
	if manager.ShareCount() != 1 || manager.shares["windows"] != "/images/first" {
		t.Fatalf("original share changed: %v", manager.shares)
	}
}
