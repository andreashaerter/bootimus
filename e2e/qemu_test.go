package e2e

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	loaderBIOS = "undionly.kpxe"
	loaderUEFI = "bootimus.efi"

	serialBanner      = "Bootimus iPXE bootloader"
	serialMenuTitle   = "Bootimus - Boot Menu"
	serialNoBootUEFI  = "No bootable option or device was found"
	serialPXEFailed   = "failed to load Boot"
	serialAlpineInit  = "Alpine Init"
	serialLinuxBanner = "Linux version"
)

type firmware struct {
	code string
	vars string
}

var ovmfCandidates = []firmware{
	{"/usr/share/OVMF/OVMF_CODE_4M.fd", "/usr/share/OVMF/OVMF_VARS_4M.fd"},
	{"/usr/share/OVMF/OVMF_CODE.fd", "/usr/share/OVMF/OVMF_VARS.fd"},
	{"/usr/share/edk2-ovmf/x64/OVMF_CODE.4m.fd", "/usr/share/edk2-ovmf/x64/OVMF_VARS.4m.fd"},
	{"/usr/share/edk2-ovmf/x64/OVMF_CODE.fd", "/usr/share/edk2-ovmf/x64/OVMF_VARS.fd"},
	{"/usr/share/edk2/ovmf/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_VARS.fd"},
	{"/usr/share/edk2/x64/OVMF_CODE.4m.fd", "/usr/share/edk2/x64/OVMF_VARS.4m.fd"},
	{"/usr/share/qemu/ovmf-x86_64-code.bin", "/usr/share/qemu/ovmf-x86_64-vars.bin"},
}

func findOVMF() (firmware, bool) {
	for _, c := range ovmfCandidates {
		if _, err := os.Stat(c.code); err != nil {
			continue
		}
		if _, err := os.Stat(c.vars); err != nil {
			continue
		}
		return c, true
	}
	return firmware{}, false
}

func qemuBinary(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		t.Fatalf("qemu-system-x86_64 not found on PATH: install the qemu-system-x86 package")
	}
	return path
}

type vmOptions struct {
	name     string
	mac      string
	uefi     bool
	bootfile string
	tftpDir  string
	fatDir   string
	memory   string
}

type vm struct {
	t          *testing.T
	name       string
	cmd        *exec.Cmd
	serialPath string
	done       chan error
}

func (h *harness) serconFile() string {
	t := h.t
	t.Helper()
	path := filepath.Join(h.cfg.runDir, "sercon-port.bin")
	if _, err := os.Stat(path); err != nil {
		if err := os.WriteFile(path, []byte{0xf8, 0x03, 0x00, 0x00}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func (h *harness) startVM(o vmOptions) *vm {
	t := h.t
	t.Helper()
	dir := filepath.Join(h.cfg.runDir, o.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	v := &vm{t: t, name: o.name, serialPath: filepath.Join(dir, "serial.log"), done: make(chan error, 1)}
	memory := o.memory
	if memory == "" {
		memory = "1G"
	}
	args := []string{
		"-m", memory,
		"-machine", "q35",
		"-display", "none",
		"-vga", "none",
		"-no-reboot",
		"-serial", "file:" + v.serialPath,
	}
	if h.cfg.kvm {
		args = append(args, "-enable-kvm", "-cpu", "host")
	}
	nic := "virtio-net-pci,netdev=n0,mac=" + o.mac
	if o.fatDir == "" {
		nic += ",bootindex=0"
	}
	args = append(args, "-netdev", h.net.qemuNetdev(o.tftpDir, o.bootfile), "-device", nic)
	if o.uefi {
		fw, ok := findOVMF()
		if !ok {
			t.Fatalf("no OVMF firmware found (looked for %v); install the ovmf or edk2-ovmf package", ovmfCandidates)
		}
		vars := filepath.Join(dir, "ovmf-vars.fd")
		if err := copyFile(fw.vars, vars); err != nil {
			t.Fatal(err)
		}
		args = append(args,
			"-drive", "if=pflash,format=raw,readonly=on,file="+fw.code,
			"-drive", "if=pflash,format=raw,file="+vars,
		)
		if o.fatDir != "" {
			args = append(args,
				"-drive", "file=fat:rw:"+o.fatDir+",format=raw,if=none,id=fat",
				"-device", "virtio-blk-pci,drive=fat,bootindex=0",
			)
		}
	} else {
		args = append(args, "-fw_cfg", "name=etc/sercon-port,file="+h.serconFile(), "-boot", "n")
	}
	stdout, err := os.Create(filepath.Join(dir, "qemu.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(qemuBinary(t), args...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("start qemu: %v", err)
	}
	v.cmd = cmd
	go func() {
		v.done <- cmd.Wait()
		stdout.Close()
	}()
	t.Cleanup(v.stop)
	t.Logf("%s: qemu pid %d (%s)", o.name, cmd.Process.Pid, strings.Join(args, " "))
	return v
}

func (v *vm) stop() {
	if v.cmd == nil || v.cmd.Process == nil {
		return
	}
	select {
	case err := <-v.done:
		v.done <- err
		return
	default:
	}
	v.cmd.Process.Signal(syscall.SIGTERM)
	var err error
	select {
	case err = <-v.done:
	case <-time.After(10 * time.Second):
		v.cmd.Process.Kill()
		err = <-v.done
	}
	v.done <- err
}

func (v *vm) exited() bool {
	select {
	case err := <-v.done:
		v.done <- err
		return true
	default:
		return false
	}
}

func (v *vm) serialContains(substr string) bool {
	return fileContains(v.serialPath, substr)
}

func (v *vm) serialTail() string {
	return truncate(tail(v.serialPath, 15), 1500)
}

func (h *harness) stagingDir(b *bootimus) string {
	t := h.t
	t.Helper()
	if !b.tftpByQEMU {
		return ""
	}
	if b.stagingDir != "" {
		return b.stagingDir
	}
	dir := filepath.Join(h.cfg.runDir, b.name, "tftp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{loaderBIOS, loaderUEFI} {
		if err := copyFile(filepath.Join(h.cfg.repoRoot, "bootloaders", "default", name), filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	autoexec, err := httpGet(b.bootURL + "/autoexec.ipxe")
	if err != nil {
		t.Fatalf("fetch autoexec.ipxe: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "autoexec.ipxe"), autoexec, 0o644); err != nil {
		t.Fatal(err)
	}
	b.stagingDir = dir
	return dir
}

func (h *harness) fatDirWith(name, efiPath string) string {
	t := h.t
	t.Helper()
	dir := filepath.Join(h.cfg.runDir, name, "fat")
	if err := os.MkdirAll(filepath.Join(dir, "EFI", "BOOT"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(efiPath, filepath.Join(dir, "EFI", "BOOT", "BOOTX64.EFI")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
