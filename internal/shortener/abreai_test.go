package shortener_test

import (
	"strings"
	"testing"

	"github.com/wiz/sendsmtp/internal/shortener"
)

func TestUniqueDestination(t *testing.T) {
	u := shortener.UniqueDestination("https://example.com/path")
	if !strings.Contains(u, "example.com") || !strings.Contains(u, "_=") {
		t.Fatalf("%s", u)
	}
}

func TestShortenLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := shortener.New()
	link, err := c.Shorten("https://example.com/sendsmtp-test")
	if err != nil {
		t.Skipf("abre.ai unavailable: %v", err)
	}
	if !strings.Contains(link, "abre.ai/") {
		t.Fatalf("%s", link)
	}
}
