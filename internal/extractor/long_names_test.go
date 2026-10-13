package extractor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const longRepodataName = "3ecd400e6569a8336ea642478904a977b0139b5a79936748142ef64cbe788bb2-primary.xml.gz"

func buildJolietOnlyISO(t *testing.T, isoPath string, files map[string]string) {
	t.Helper()
	xorriso, err := exec.LookPath("xorriso")
	if err != nil {
		t.Skip("xorriso not installed")
	}
	if _, err := exec.LookPath("bsdtar"); err != nil {
		t.Skip("bsdtar not installed")
	}

	src := t.TempDir()
	for path, content := range files {
		full := filepath.Join(src, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command(xorriso, "-rockridge", "off", "-joliet", "on", "-compliance", "joliet_long_names",
		"-outdev", isoPath, "-map", src, "/", "-commit")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xorriso: %v\n%s", err, out)
	}
}

func assertExtractedTree(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "repodata")
	file := filepath.Join(dir, longRepodataName)
	if data, err := os.ReadFile(file); err != nil || string(data) != "primary" {
		t.Fatalf("long repodata name not preserved: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm()&0755 != 0755 {
		t.Errorf("repodata directory mode = %v, want at least 0755", info.Mode().Perm())
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm()&0644 != 0644 {
		t.Errorf("repodata file mode = %v, want at least 0644", info.Mode().Perm())
	}
}

func TestExtractKeepsLongJolietNames(t *testing.T) {
	dataDir := t.TempDir()
	isoPath := filepath.Join(dataDir, "Kylin-Server-10-SP1.iso")
	buildJolietOnlyISO(t, isoPath, map[string]string{
		"images/pxeboot/vmlinuz":       "kernel",
		"images/pxeboot/initrd.img":    "initrd",
		"repodata/repomd.xml":          "repomd",
		"repodata/" + longRepodataName: "primary",
	})

	ext, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ext.SetProgress(NewProgressReporter())
	bootFiles, err := ext.Extract(isoPath)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if bootFiles.ExtractedDir == "" {
		t.Fatal("expected the full ISO tree to be extracted")
	}
	assertExtractedTree(t, bootFiles.ExtractedDir)
}

func TestExtractTreeKeepsLongJolietNames(t *testing.T) {
	isoPath := filepath.Join(t.TempDir(), "tool.iso")
	buildJolietOnlyISO(t, isoPath, map[string]string{
		"repodata/" + longRepodataName: "primary",
	})

	destDir := t.TempDir()
	if err := ExtractTree(isoPath, destDir); err != nil {
		t.Fatalf("ExtractTree: %v", err)
	}
	assertExtractedTree(t, destDir)
}
