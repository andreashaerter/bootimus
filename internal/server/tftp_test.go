package server

import (
	"testing"
	"unicode/utf8"
)

func TestSanitiseTFTPFilename(t *testing.T) {
	cases := map[string]string{
		"bootimus.efi":                       "bootimus.efi",
		"7efcf632/start4.elf":                "7efcf632/start4.elf",
		"bootimus.efi\xff Linux/5.10.66_s5":  "bootimus.efi",
		"bootimus.efi\x00":                   "bootimus.efi",
		"undionly.kpxe\r\nX-User-Agent: foo": "undionly.kpxe",
		"\xffbootimus.efi":                   "",
	}
	for in, want := range cases {
		got := sanitiseTFTPFilename(in)
		if got != want {
			t.Errorf("sanitiseTFTPFilename(%q) = %q, want %q", in, got, want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("sanitiseTFTPFilename(%q) returned invalid UTF-8 %q", in, got)
		}
	}
}
