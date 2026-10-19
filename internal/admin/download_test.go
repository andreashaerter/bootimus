package admin

import "testing"

func TestISODownloadFilename(t *testing.T) {
	cases := []struct {
		url, requested, want string
		wantErr              bool
	}{
		{url: "https://example.com/pub/ubuntu.iso", want: "ubuntu.iso"},
		{url: "https://example.com/pub/ubuntu.iso", requested: "renamed.ISO", want: "renamed.ISO"},
		{url: "https://example.com/pub/readme.txt", wantErr: true},
		{url: "https://github.com/rgl/hello-world-debian-uki/releases/download/v0.0.1/hello-world.efi", want: "hello-world.efi"},
		{url: "https://example.com/pub/ubuntu.iso", requested: "hello.efi", want: "hello.efi"},
		{url: "https://example.com/pub/ubuntu.iso", requested: "renamed.img", wantErr: true},
		{url: "https://example.com/pub/ubuntu.iso", requested: "../../etc/evil.iso", wantErr: true},
		{url: "https://example.com/pub/ubuntu.iso", requested: "sub/dir.iso", wantErr: true},
		{url: "https://example.com/pub/ubuntu.iso", requested: `..\evil.iso`, wantErr: true},
		{url: "https://example.com/pub/ubuntu.iso", requested: "..", wantErr: true},
	}
	for _, c := range cases {
		got, err := isoDownloadFilename(c.url, c.requested)
		if c.wantErr {
			if err == nil {
				t.Errorf("isoDownloadFilename(%q, %q) = %q, want error", c.url, c.requested, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("isoDownloadFilename(%q, %q) = %q, %v; want %q", c.url, c.requested, got, err, c.want)
		}
	}
}
