package emailvalid

import (
	"context"
	"testing"
	"time"
)

func TestSyntaxOK(t *testing.T) {
	good := []string{"a@b.co", "user.name+tag@example.com"}
	bad := []string{"", "nodomain", "@x.com", "a@.com", "a@localhost", "a@127.0.0.1"}
	for _, e := range good {
		if !SyntaxOK(Normalize(e)) {
			t.Fatalf("expected ok: %s", e)
		}
	}
	for _, e := range bad {
		if SyntaxOK(Normalize(e)) {
			t.Fatalf("expected bad: %s", e)
		}
	}
}

func TestFilterDedupAndMX(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	in := []string{
		"Valid@Gmail.com",
		"valid@gmail.com", // dupe
		"bad@@nope",
		"nobody@this-domain-should-not-exist-zzz-9f3a1.example",
	}
	r := Filter(ctx, in, Options{Workers: 8, Timeout: 5 * time.Second, RequireMX: true})
	if r.Duplicate != 1 {
		t.Fatalf("dup=%d want 1", r.Duplicate)
	}
	if len(r.Valid) < 1 {
		t.Fatalf("expected gmail valid, got %#v", r)
	}
	found := false
	for _, v := range r.Valid {
		if v == "valid@gmail.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gmail missing: %#v", r.Valid)
	}
}

func TestDisposableRejected(t *testing.T) {
	if !IsDisposable("mailinator.com") {
		t.Fatal("expected disposable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := Filter(ctx, []string{"a@mailinator.com"}, Options{RequireMX: true, Workers: 1, Timeout: time.Second})
	if len(r.Valid) != 0 || len(r.Invalid) != 1 {
		t.Fatalf("want disposable invalid: %#v", r)
	}
}
