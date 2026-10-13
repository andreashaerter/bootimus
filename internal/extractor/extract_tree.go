package extractor

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kdomanski/iso9660"
)

func ExtractTree(isoPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	tarErr := extractWithBsdtar(isoPath, destDir, nil)
	if tarErr == nil {
		return nil
	}

	log.Printf("bsdtar tree extraction unavailable or failed (%v), using the built-in ISO9660 reader; long file names may be shortened", tarErr)
	if err := extractTreeViaISO9660(isoPath, destDir); err != nil {
		return fmt.Errorf("iso9660 tree extraction failed: %w (bsdtar: %v)", err, tarErr)
	}
	return nil
}

func extractWithBsdtar(isoPath, destDir string, progress *ProgressReporter) error {
	bsdtarPath, err := exec.LookPath("bsdtar")
	if err != nil {
		return err
	}

	cmd := exec.Command(bsdtarPath, "-xvf", isoPath, "-C", destDir)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var messages []string
	previous := ""
	entries := 0
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, "x "); ok {
			addExtractedSize(progress, destDir, previous)
			previous = name
			if name != "." && name != "./" {
				entries++
			}
			continue
		}
		if len(messages) < 5 {
			messages = append(messages, line)
		}
	}
	addExtractedSize(progress, destDir, previous)

	waitErr := cmd.Wait()
	makeTreeWritable(destDir)
	if waitErr != nil {
		return fmt.Errorf("bsdtar failed: %w (%s)", waitErr, strings.Join(messages, "; "))
	}
	if entries == 0 {
		return fmt.Errorf("bsdtar extracted nothing from %s", isoPath)
	}
	return nil
}

func addExtractedSize(progress *ProgressReporter, destDir, name string) {
	if progress == nil || name == "" {
		return
	}
	if info, err := os.Lstat(filepath.Join(destDir, name)); err == nil && info.Mode().IsRegular() {
		progress.AddBytes(info.Size())
	}
}

func makeTreeWritable(root string) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		want := info.Mode().Perm() | 0644
		if d.IsDir() {
			want = info.Mode().Perm() | 0755
		} else if !info.Mode().IsRegular() {
			return nil
		}
		if want != info.Mode().Perm() {
			os.Chmod(path, want)
		}
		return nil
	})
}

func extractTreeViaISO9660(isoPath, destDir string) error {
	f, err := os.Open(isoPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := iso9660.OpenImage(f)
	if err != nil {
		return err
	}
	root, err := img.RootDir()
	if err != nil {
		return err
	}
	return extractTreeNode(root, destDir, destDir)
}

func extractTreeNode(node *iso9660.File, target, rootDir string) error {
	if target != rootDir && !strings.HasPrefix(filepath.Clean(target), filepath.Clean(rootDir)+string(os.PathSeparator)) {
		return nil
	}

	if node.IsDir() {
		if err := os.MkdirAll(target, 0755); err != nil {
			return err
		}
		children, err := safeGetChildren(node)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := extractTreeNode(child, filepath.Join(target, child.Name()), rootDir); err != nil {
				return err
			}
		}
		return nil
	}

	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, node.Reader())
	return err
}
