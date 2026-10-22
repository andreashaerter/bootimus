package extractor

import "testing"

func TestExtractInstallerLayouts(t *testing.T) {
	tests := []struct {
		isoName        string
		files          map[string]string
		wantDistro     string
		wantKernel     string
		wantInitrd     string
		wantNetboot    bool
		wantNetbootURL string
	}{
		{
			isoName: "ubuntu-18.04.6-server-amd64.iso",
			files: map[string]string{
				"/install/vmlinuz":                                  "cdrom-kernel",
				"/install/initrd.gz":                                "cdrom-initrd",
				"/install/netboot/ubuntu-installer/amd64/linux":     "netboot-kernel",
				"/install/netboot/ubuntu-installer/amd64/initrd.gz": "netboot-initrd",
				"/.disk/info":                                       `Ubuntu-Server 18.04.6 LTS "Bionic Beaver" - Release amd64 (20210915.2)`,
			},
			wantDistro: "ubuntu",
			wantKernel: "netboot-kernel",
			wantInitrd: "netboot-initrd",
		},
		{
			isoName: "debian-13.7.0-amd64-netinst.iso",
			files: map[string]string{
				"/install.amd/vmlinuz":   "installer-kernel",
				"/install.amd/initrd.gz": "installer-initrd",
				"/.disk/info":            `Debian GNU/Linux 13.7.0 "Trixie" - Official amd64 NETINST with firmware 20260912-09:35`,
			},
			wantDistro:     "debian",
			wantKernel:     "installer-kernel",
			wantInitrd:     "installer-initrd",
			wantNetboot:    true,
			wantNetbootURL: "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.isoName, func(t *testing.T) {
			bootFiles, _ := extractTestISO(t, tt.isoName, tt.files)

			if bootFiles.Distro != tt.wantDistro {
				t.Errorf("distro = %q, want %q", bootFiles.Distro, tt.wantDistro)
			}
			if got := readFileString(t, bootFiles.Kernel); got != tt.wantKernel {
				t.Errorf("extracted kernel content = %q, want %q", got, tt.wantKernel)
			}
			if got := readFileString(t, bootFiles.Initrd); got != tt.wantInitrd {
				t.Errorf("extracted initrd content = %q, want %q", got, tt.wantInitrd)
			}
			if bootFiles.NetbootRequired != tt.wantNetboot {
				t.Errorf("netboot required = %v, want %v", bootFiles.NetbootRequired, tt.wantNetboot)
			}
			if bootFiles.NetbootURL != tt.wantNetbootURL {
				t.Errorf("netboot URL = %q, want %q", bootFiles.NetbootURL, tt.wantNetbootURL)
			}
		})
	}
}
