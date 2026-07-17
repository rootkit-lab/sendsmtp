package parser_test

import (
	"testing"

	"github.com/wiz/sendsmtp/internal/parser"
)

func TestParseGoscanSMTPs(t *testing.T) {
	raw := `--- SMTP config (goscan) ---
domain: bitvortax.com
account_label: MAIL/SMTP
host: mail.bitvortax.com
port: 587
encryption: tls
from: info@bitvortax.com
user: info@bitvortax.com
password: secret

--- SMTP config (goscan) ---
domain: other.com
host: smtp.other.com
port: 465
encryption: ssl
from: a@other.com
user: a@other.com
password: x
`
	accs, err := parser.ParseGoscanSMTPs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 2 {
		t.Fatalf("want 2, got %d", len(accs))
	}
	if accs[0].Host != "mail.bitvortax.com" || accs[0].Port != 587 {
		t.Fatalf("bad first: %+v", accs[0])
	}
	if accs[1].Encryption != "ssl" {
		t.Fatalf("bad second enc: %s", accs[1].Encryption)
	}
}

func TestParseEmailPasswordLines(t *testing.T) {
	raw := `atendimento@creluz.com.br;@Creluz2026
# comment
user@example.com;secret123
host: mail.x.com
`
	accs := parser.ParseEmailPasswordLines(raw)
	if len(accs) != 2 {
		t.Fatalf("want 2, got %d: %+v", len(accs), accs)
	}
	if accs[0].User != "atendimento@creluz.com.br" || accs[0].Password != "@Creluz2026" {
		t.Fatalf("bad first: %+v", accs[0])
	}
	if accs[0].Domain != "creluz.com.br" || accs[0].From != accs[0].User {
		t.Fatalf("domain/from: %+v", accs[0])
	}
	if accs[1].Password != "secret123" {
		t.Fatalf("second pass: %+v", accs[1])
	}
}

func TestLooksLikeGoscan(t *testing.T) {
	if !parser.LooksLikeGoscan("--- SMTP config (goscan) ---\nhost: x\nuser: y") {
		t.Fatal("expected goscan")
	}
	if parser.LooksLikeGoscan("a@b.com;pass") {
		t.Fatal("credential lines should not look like goscan")
	}
}
