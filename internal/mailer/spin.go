package mailer

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"
)

// Spin expands spintax groups of the form {opt1|opt2|opt3}.
// Innermost groups are resolved first; nested spin is supported.
// Groups without "|" are left unchanged (so "{{email}}" survivors stay safe if any).
func Spin(s string) string {
	for {
		start, end := findSpinGroup(s)
		if start < 0 {
			return s
		}
		inner := s[start+1 : end]
		parts := splitSpinOptions(inner)
		pick := parts[0]
		if len(parts) > 1 {
			pick = parts[randInt(len(parts))]
		}
		s = s[:start] + pick + s[end+1:]
	}
}

// Prepare replaces placeholders then expands spintax — unique per call.
// {{link}} is always personalized as base?p=<email> (see PersonalizeLink).
func Prepare(tpl, email, link, subject, from string) string {
	link = PersonalizeLink(link, email)
	return Spin(ApplyPlaceholders(tpl, email, link, subject, from))
}

// PersonalizeLink appends or sets query p=<email> on the base URL.
// Examples: https://x.com → https://x.com/?p=a%40b.com ; https://x.com/a?y=1 → …&p=…
func PersonalizeLink(base, email string) string {
	base = strings.TrimSpace(base)
	email = strings.TrimSpace(email)
	if base == "" {
		return ""
	}
	if email == "" {
		return base
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// Non-URL fallback: ensure /?p=
		trimmed := strings.TrimRight(base, "/")
		return trimmed + "/?p=" + url.QueryEscape(email)
	}
	q := u.Query()
	q.Set("p", email)
	u.RawQuery = q.Encode()
	// Prefer trailing slash before ? when path is empty (https://host/?p=…)
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

// SanitizeFrom drops unresolved env-style placeholders (e.g. ${MAIL_USERNAME})
// so they never leak into the rendered message.
func SanitizeFrom(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	if strings.Contains(from, "${") || strings.Contains(from, "{{") {
		return ""
	}
	return from
}

// ResolveFrom prefers a real FromAddr; falls back to SMTP user when it looks like an email.
func ResolveFrom(fromAddr, user string) string {
	if f := SanitizeFrom(fromAddr); f != "" {
		return f
	}
	user = strings.TrimSpace(user)
	if strings.Contains(user, "@") && !strings.Contains(user, "${") {
		return strings.ToLower(user)
	}
	return ""
}

func ApplyPlaceholders(tpl, email, link, subject, from string) string {
	uniq := newUniq()
	from = SanitizeFrom(from)
	fromBit := ""
	if from != "" {
		fromBit = " · " + from
	}
	// Optional wrapper hides the separator when from is empty/unresolved.
	out := strings.ReplaceAll(tpl, "<span data-from>{{from}}</span>", fromBit)
	return strings.NewReplacer(
		"{{email}}", email,
		"{{link}}", link,
		"{{assunto}}", subject,
		"{{subject}}", subject,
		"{{from}}", from,
		"{{uniq}}", uniq,
		"{{id}}", uniq,
	).Replace(out)
}

func findSpinGroup(s string) (start, end int) {
	start = -1
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				inner := s[start+1 : i]
				if strings.Contains(inner, "|") {
					return start, i
				}
				// no pipe → skip this brace pair, keep searching
				start = -1
			}
		}
	}
	return -1, -1
}

func splitSpinOptions(inner string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == 0 {
				parts = append(parts, inner[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, inner[start:])
	if len(parts) == 0 {
		return []string{inner}
	}
	return parts
}

var (
	uniqMu sync.Mutex
	uniqN  uint64
)

func newUniq() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	uniqMu.Lock()
	uniqN++
	n := uniqN
	uniqMu.Unlock()
	return hex.EncodeToString(b[:]) + formatHex(n)
}

func formatHex(n uint64) string {
	const hexdigits = "0123456789abcdef"
	var buf [4]byte
	for i := 3; i >= 0; i-- {
		buf[i] = hexdigits[n&0xf]
		n >>= 4
	}
	return string(buf[:])
}

func randInt(n int) int {
	if n <= 1 {
		return 0
	}
	var b [1]byte
	_, _ = rand.Read(b[:])
	return int(b[0]) % n
}
