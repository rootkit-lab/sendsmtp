package emailvalid

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	// Practical address check (not full RFC 5322).
	addrRe = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)
)

// Common disposable / throwaway domains — reject when validating.
var disposableDomains = map[string]struct{}{
	"mailinator.com": {}, "guerrillamail.com": {}, "guerrillamail.net": {},
	"sharklasers.com": {}, "grr.la": {}, "yopmail.com": {}, "yopmail.fr": {},
	"tempmail.com": {}, "temp-mail.org": {}, "throwawaymail.com": {},
	"10minutemail.com": {}, "10minemail.com": {}, "trashmail.com": {},
	"trashmail.me": {}, "fakeinbox.com": {}, "getnada.com": {},
	"maildrop.cc": {}, "dispostable.com": {}, "mailnesia.com": {},
	"tempail.com": {}, "emailondeck.com": {}, "moakt.com": {},
	"mohmal.com": {}, "discard.email": {}, "mailcatch.com": {},
	"mailnull.com": {}, "spamgourmet.com": {}, "mailinator.net": {},
	"guerrillamailblock.com": {}, "spam4.me": {}, "tmpmail.org": {},
	"tmpmail.net": {}, "emailfake.com": {}, "crazymailing.com": {},
}

type Options struct {
	Workers       int                   // concurrent domain checks (default 6)
	Timeout       time.Duration         // per-domain DNS budget (default 3s)
	Pace          time.Duration         // min gap between starting DNS lookups (default 25ms)
	ProgressEvery time.Duration         // throttle OnProgress (default 200ms; always fires on last)
	RequireMX     bool                  // if true (default), domain must have resolvable MX — not only A
	OnProgress    func(done, total int) // optional; called as domains finish DNS
}

type Result struct {
	Valid      []string
	Invalid    []string
	Duplicate  int // duplicates within the input (after normalize)
	DomainsOK  int
	DomainsBad int
}

type domainVerdict struct {
	ok  bool
	err string
}

// Filter validates addresses: syntax, disposable blocklist, unique domains via MX DNS.
// Only Valid addresses should be imported. Invalid and duplicates-in-input are excluded.
func Filter(ctx context.Context, addresses []string, opt Options) Result {
	if opt.Workers <= 0 {
		opt.Workers = 6
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 3 * time.Second
	}
	if opt.Pace <= 0 {
		opt.Pace = 25 * time.Millisecond
	}
	if opt.ProgressEvery <= 0 {
		opt.ProgressEvery = 200 * time.Millisecond
	}
	// Default RequireMX=true when zero-value Options used from engine with explicit field set.
	// Callers must set RequireMX; engine sets true.

	seen := make(map[string]struct{}, len(addresses))
	var unique []string
	var res Result
	for _, raw := range addresses {
		addr := Normalize(raw)
		if addr == "" {
			res.Invalid = append(res.Invalid, strings.TrimSpace(raw))
			continue
		}
		if !SyntaxOK(addr) {
			res.Invalid = append(res.Invalid, addr)
			continue
		}
		if IsDisposable(DomainOf(addr)) {
			res.Invalid = append(res.Invalid, addr)
			continue
		}
		if _, ok := seen[addr]; ok {
			res.Duplicate++
			continue
		}
		seen[addr] = struct{}{}
		unique = append(unique, addr)
	}

	domainOf := make(map[string]string, len(unique))
	domains := make([]string, 0)
	domSeen := make(map[string]struct{})
	for _, addr := range unique {
		dom := DomainOf(addr)
		domainOf[addr] = dom
		if _, ok := domSeen[dom]; ok {
			continue
		}
		domSeen[dom] = struct{}{}
		domains = append(domains, dom)
	}

	verdicts := checkDomains(ctx, domains, opt)

	for _, d := range domains {
		if verdicts[d].ok {
			res.DomainsOK++
		} else {
			res.DomainsBad++
		}
	}

	for _, addr := range unique {
		dom := domainOf[addr]
		if v, ok := verdicts[dom]; ok && v.ok {
			res.Valid = append(res.Valid, addr)
		} else {
			res.Invalid = append(res.Invalid, addr)
		}
	}
	return res
}

func Normalize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.Trim(s, `"'<>`)
	s = strings.TrimSpace(s)
	return s
}

func IsDisposable(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	_, ok := disposableDomains[domain]
	return ok
}

func SyntaxOK(addr string) bool {
	if len(addr) < 6 || len(addr) > 254 {
		return false
	}
	at := strings.LastIndex(addr, "@")
	if at < 1 || at == len(addr)-1 {
		return false
	}
	local := addr[:at]
	if len(local) > 64 || len(local) < 1 || strings.Contains(local, "..") {
		return false
	}
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") {
		return false
	}
	dom := addr[at+1:]
	if strings.HasPrefix(dom, ".") || strings.HasSuffix(dom, ".") || strings.Contains(dom, "..") {
		return false
	}
	if strings.Contains(dom, "--") {
		return false
	}
	labels := strings.Split(dom, ".")
	if len(labels) < 2 {
		return false
	}
	tld := labels[len(labels)-1]
	if len(tld) < 2 || !isAlpha(tld) {
		return false
	}
	if net.ParseIP(strings.Trim(dom, "[]")) != nil {
		return false
	}
	if dom == "localhost" || strings.HasSuffix(dom, ".local") || strings.HasSuffix(dom, ".test") ||
		strings.HasSuffix(dom, ".invalid") || strings.HasSuffix(dom, ".example") {
		return false
	}
	return addrRe.MatchString(addr)
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func DomainOf(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return ""
	}
	return addr[at+1:]
}

func checkDomains(ctx context.Context, domains []string, opt Options) map[string]domainVerdict {
	out := make(map[string]domainVerdict, len(domains))
	if len(domains) == 0 {
		return out
	}

	var mu sync.Mutex
	var doneCount int
	var lastProgress time.Time
	total := len(domains)

	mark := func(dom, reason string, ok bool) {
		var cb func(done, total int)
		var n int
		mu.Lock()
		out[dom] = domainVerdict{ok: ok, err: reason}
		doneCount++
		n = doneCount
		if opt.OnProgress != nil && (n == total || lastProgress.IsZero() || time.Since(lastProgress) >= opt.ProgressEvery) {
			lastProgress = time.Now()
			cb = opt.OnProgress
		}
		mu.Unlock()
		if cb != nil {
			cb(n, total)
		}
	}

	jobs := make(chan string, len(domains))
	for _, d := range domains {
		jobs <- d
	}
	close(jobs)

	pace := time.NewTicker(opt.Pace)
	defer pace.Stop()

	workers := opt.Workers
	if workers > len(domains) {
		workers = len(domains)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dom := range jobs {
				if ctx.Err() != nil {
					mark(dom, ctx.Err().Error(), false)
					for dom := range jobs {
						mark(dom, ctx.Err().Error(), false)
					}
					return
				}
				select {
				case <-ctx.Done():
					mark(dom, ctx.Err().Error(), false)
					for dom := range jobs {
						mark(dom, ctx.Err().Error(), false)
					}
					return
				case <-pace.C:
				}
				ok, reason := domainAcceptsMail(ctx, dom, opt.Timeout, opt.RequireMX)
				mark(dom, reason, ok)
			}
		}()
	}
	wg.Wait()
	return out
}

// domainAcceptsMail: requires usable MX (and MX host that resolves) when requireMX.
// Without requireMX, falls back to A/AAAA (implicit MX) for legacy behaviour.
func domainAcceptsMail(ctx context.Context, domain string, timeout time.Duration, requireMX bool) (bool, string) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" || !strings.Contains(domain, ".") {
		return false, "invalid domain"
	}
	if IsDisposable(domain) {
		return false, "disposable"
	}

	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resolver := &net.Resolver{}

	mxs, err := resolver.LookupMX(dctx, domain)
	if err == nil && len(mxs) > 0 {
		var hasNullOnly = true
		for _, mx := range mxs {
			host := strings.TrimSuffix(strings.ToLower(mx.Host), ".")
			if host == "" || host == "." {
				continue
			}
			hasNullOnly = false
			// Prefer MX that resolves — proves mail path is real.
			ipctx, ipcancel := context.WithTimeout(dctx, timeout/2+500*time.Millisecond)
			addrs, aerr := resolver.LookupIPAddr(ipctx, host)
			ipcancel()
			if aerr == nil && len(addrs) > 0 {
				return true, ""
			}
		}
		if hasNullOnly {
			return false, "null mx"
		}
		// MX exists but hosts didn't resolve in time — still accept (common with rate-limited DNS).
		return true, ""
	}
	if isNotFound(err) {
		return false, "nxdomain"
	}

	if requireMX {
		if err != nil {
			return false, err.Error()
		}
		return false, "no mx"
	}

	// Legacy fallback: A/AAAA implicit MX
	if dctx.Err() != nil {
		if err != nil {
			return false, err.Error()
		}
		return false, dctx.Err().Error()
	}
	addrs, aerr := resolver.LookupIPAddr(dctx, domain)
	if aerr == nil && len(addrs) > 0 {
		return true, ""
	}
	if isNotFound(aerr) {
		return false, "nxdomain"
	}
	if aerr != nil {
		return false, aerr.Error()
	}
	if err != nil {
		return false, err.Error()
	}
	return false, "no mx/a"
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such host") || strings.Contains(msg, "nxdomain")
}
