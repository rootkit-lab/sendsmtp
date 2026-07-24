package serverparse_test

import (
	"testing"

	"github.com/wiz/sendsmtp/internal/serverparse"
)

func TestParsePipeIPPassword(t *testing.T) {
	raw := `|23.27.96.206|l7Bff3p3i0F4davedbT6
|50.114.114.33|FbV8o5L5k3effab5vcYa
`
	list, invalid := serverparse.ParseLines(raw)
	if invalid != 0 {
		t.Fatalf("invalid=%d", invalid)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].Host != "23.27.96.206" || list[0].SSHPassword == "" {
		t.Fatalf("%+v", list[0])
	}
	if list[0].PreferPort != 18080 {
		t.Fatalf("prefer=%d", list[0].PreferPort)
	}
}

func TestParseSOCKSLine(t *testing.T) {
	list, invalid := serverparse.ParseLines("1.2.3.4:1080:user:pass")
	if invalid != 0 || len(list) != 1 {
		t.Fatalf("%d %d", invalid, len(list))
	}
	if list[0].ProxyPort != 1080 || list[0].Status != "active" {
		t.Fatalf("%+v", list[0])
	}
}
