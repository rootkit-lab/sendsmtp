package smtpdiscover

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestKnownProviderCandidates(t *testing.T) {
	ClearCache()
	cands := buildCandidates(context.Background(), "gmail.com")
	if len(cands) < 1 || cands[0].Host != "smtp.gmail.com" {
		t.Fatalf("expected gmail smtp first, got %#v", cands)
	}
	if len(cands) > 4 {
		t.Fatalf("known provider should be few candidates, got %d", len(cands))
	}
}

func TestBuildCandidatesCustomDomain(t *testing.T) {
	cands := buildCandidates(context.Background(), "creluz.com.br")
	found := false
	for _, c := range cands {
		if c.Host == "smtp.creluz.com.br" && c.Port == 587 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing smtp.creluz.com.br:587 in %#v", cands)
	}
	if len(cands) > 14 {
		t.Fatalf("too many candidates: %d", len(cands))
	}
}

func TestLocawebMXMapsToEmailSSL(t *testing.T) {
	hosts := submissionHostsFromMX([]string{"mx.core.locaweb.com.br", "mx.b.locaweb.com.br"})
	if len(hosts) < 1 || hosts[0] != "email-ssl.com.br" {
		t.Fatalf("want email-ssl first, got %#v", hosts)
	}
	if isInboundMXHost("mx.core.locaweb.com.br") != true {
		t.Fatal("mx.core should be inbound")
	}
}

func TestBuildCandidatesAzzultechPrefersEmailSSL(t *testing.T) {
	ClearCache()
	cands := buildCandidates(context.Background(), "azzultech.com.br")
	if len(cands) == 0 {
		t.Fatal("no candidates")
	}
	if cands[0].Host != "email-ssl.com.br" {
		t.Fatalf("first candidate want email-ssl.com.br, got %#v", cands[:min(4, len(cands))])
	}
	// Must not waste probes on inbound mx.*
	for _, c := range cands {
		if strings.HasPrefix(c.Host, "mx.") {
			t.Fatalf("should not probe inbound MX %s", c.Host)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestLiveAzzultechLocaweb(t *testing.T) {
	pass := os.Getenv("SENDSMTP_TEST_PASS")
	if pass == "" {
		t.Skip("set SENDSMTP_TEST_PASS to run live AUTH")
	}
	ClearCache()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := Discover(ctx, "azzultech.com.br", "alberto.santos@azzultech.com.br", pass, Options{
		ProbeTimeout: 4 * time.Second,
		Workers:      4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Host != "email-ssl.com.br" {
		t.Fatalf("want email-ssl.com.br, got %+v", r)
	}
	t.Logf("ok %+v", r)
}
