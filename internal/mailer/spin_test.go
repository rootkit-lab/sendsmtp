package mailer

import (
	"strings"
	"testing"
	"time"
)

func TestSpinBasic(t *testing.T) {
	out := Spin("{A|A|A}")
	if out != "A" {
		t.Fatalf("got %q", out)
	}
}

func TestSpinNested(t *testing.T) {
	out := Spin("{X|{Y|Y}}")
	if out != "X" && out != "Y" {
		t.Fatalf("got %q", out)
	}
}

func TestSpinIgnoresNoPipe(t *testing.T) {
	in := "hello {world} ok"
	if Spin(in) != in {
		t.Fatalf("should ignore braces without |")
	}
}

func TestPersonalizeLink(t *testing.T) {
	got := PersonalizeLink("https://baixepedido.online/", "a@b.com")
	if got != "https://baixepedido.online/?p=a%40b.com" {
		t.Fatalf("got %q", got)
	}
	got = PersonalizeLink("https://baixepedido.online", "a@b.com")
	if got != "https://baixepedido.online/?p=a%40b.com" {
		t.Fatalf("no slash: %q", got)
	}
	got = PersonalizeLink("https://x.com/path?y=1", "u@z.co")
	if !strings.Contains(got, "p=u%40z.co") || !strings.Contains(got, "y=1") {
		t.Fatalf("merge query: %q", got)
	}
	// Idempotent when Prepare calls PersonalizeLink twice
	once := PersonalizeLink("https://x.com/", "a@b.com")
	twice := PersonalizeLink(once, "a@b.com")
	if once != twice {
		t.Fatalf("idempotent want %q got %q", once, twice)
	}
}

func TestPrepareLinkHasP(t *testing.T) {
	out := Prepare(`<a href="{{link}}">x</a>`, "office@gmail.com", "https://baixepedido.online/", "s", "from@x.com")
	if !strings.Contains(out, "https://baixepedido.online/?p=office%40gmail.com") {
		t.Fatalf("missing personalized link: %s", out)
	}
}

func TestSanitizeFrom(t *testing.T) {
	if SanitizeFrom("${MAIL_USERNAME}") != "" {
		t.Fatal("expected empty for env placeholder")
	}
	if SanitizeFrom("a@b.com") != "a@b.com" {
		t.Fatal("expected keep real from")
	}
	if ResolveFrom("${MAIL_USERNAME}", "user@host.com") != "user@host.com" {
		t.Fatal("expected fallback to user")
	}
}

func TestPrepareUnique(t *testing.T) {
	tpl := "{Oi|Oi} {{uniq}} {{email}}"
	a := Prepare(tpl, "a@x.com", "https://x.com", "subj", "from@x.com")
	b := Prepare(tpl, "a@x.com", "https://x.com", "subj", "from@x.com")
	if !strings.Contains(a, "a@x.com") {
		t.Fatalf("missing email: %s", a)
	}
	if a == b {
		t.Fatalf("expected unique renders, both=%q", a)
	}
}

func TestFromWrapperHiddenWhenEmpty(t *testing.T) {
	tpl := `auto<span data-from>{{from}}</span> · {{uniq}}`
	out := ApplyPlaceholders(tpl, "a@x.com", "https://x", "s", "${MAIL_USERNAME}")
	if strings.Contains(out, "${") || strings.Contains(out, "MAIL_USERNAME") {
		t.Fatalf("leaked placeholder: %s", out)
	}
	if strings.Contains(out, " ·  · ") {
		t.Fatalf("double sep: %s", out)
	}
}

func TestDatePlaceholders(t *testing.T) {
	want := FormatDateBR(time.Now())
	out := ApplyPlaceholders("emitida em {{data}} / {{date}}", "a@x.com", "https://x", "s", "from@x.com")
	if !strings.Contains(out, want) {
		t.Fatalf("missing date %q in %q", want, out)
	}
	if strings.Contains(out, "{{data}}") || strings.Contains(out, "{{date}}") {
		t.Fatalf("unreplaced date placeholder: %q", out)
	}
}

func TestFormatDateBR(t *testing.T) {
	got := FormatDateBR(time.Date(2026, 7, 24, 15, 0, 0, 0, time.UTC))
	if got != "24/07/2026" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitNestedPipe(t *testing.T) {
	parts := splitSpinOptions("a|{b|c}|d")
	if len(parts) != 3 || parts[0] != "a" || parts[1] != "{b|c}" || parts[2] != "d" {
		t.Fatalf("%#v", parts)
	}
}
