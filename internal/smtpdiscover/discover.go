package smtpdiscover

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// Result is a discovered outbound SMTP submission endpoint.
type Result struct {
	Host       string
	Port       int
	Encryption string // starttls | ssl | tls
}

type candidate struct {
	Host       string
	Port       int
	Encryption string
}

type Options struct {
	// OnProgress is called with human-readable status (host:port).
	OnProgress func(msg string)
	// ProbeTimeout per dial+AUTH attempt (default 2.5s).
	ProbeTimeout time.Duration
	// Workers concurrent probes (default 4).
	Workers int
}

var (
	cacheMu sync.Mutex
	cache   = map[string]Result{} // domain → result
)

// knownProviders maps email domain → submission server (no network).
var knownProviders = map[string]Result{
	"gmail.com":      {Host: "smtp.gmail.com", Port: 587, Encryption: "starttls"},
	"googlemail.com": {Host: "smtp.gmail.com", Port: 587, Encryption: "starttls"},
	"outlook.com":    {Host: "smtp.office365.com", Port: 587, Encryption: "starttls"},
	"hotmail.com":    {Host: "smtp.office365.com", Port: 587, Encryption: "starttls"},
	"live.com":       {Host: "smtp.office365.com", Port: 587, Encryption: "starttls"},
	"msn.com":        {Host: "smtp.office365.com", Port: 587, Encryption: "starttls"},
	"office365.com":  {Host: "smtp.office365.com", Port: 587, Encryption: "starttls"},
	"yahoo.com":      {Host: "smtp.mail.yahoo.com", Port: 587, Encryption: "starttls"},
	"yahoo.com.br":   {Host: "smtp.mail.yahoo.com", Port: 587, Encryption: "starttls"},
	"icloud.com":     {Host: "smtp.mail.me.com", Port: 587, Encryption: "starttls"},
	"me.com":         {Host: "smtp.mail.me.com", Port: 587, Encryption: "starttls"},
	"aol.com":        {Host: "smtp.aol.com", Port: 587, Encryption: "starttls"},
	"zoho.com":       {Host: "smtp.zoho.com", Port: 587, Encryption: "starttls"},
	"protonmail.com": {Host: "smtp.protonmail.ch", Port: 587, Encryption: "starttls"},
	"proton.me":      {Host: "smtp.protonmail.ch", Port: 587, Encryption: "starttls"},
	// Locaweb direct domains (custom domains still need MX remap → email-ssl.com.br).
	"locaweb.com.br":   {Host: "email-ssl.com.br", Port: 587, Encryption: "starttls"},
	"email-ssl.com.br": {Host: "email-ssl.com.br", Port: 587, Encryption: "starttls"},
}

// Discover finds a working SMTP host for domain, verifying AUTH with user/password.
func Discover(ctx context.Context, domain, user, password string, opt Options) (Result, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	user = strings.TrimSpace(user)
	password = strings.TrimSpace(password)
	if domain == "" || !strings.Contains(domain, ".") {
		return Result{}, fmt.Errorf("invalid domain")
	}
	if user == "" || password == "" {
		return Result{}, fmt.Errorf("missing credentials")
	}
	if opt.ProbeTimeout <= 0 {
		opt.ProbeTimeout = 2500 * time.Millisecond
	}
	if opt.Workers <= 0 {
		opt.Workers = 4
	}

	report := func(msg string) {
		if opt.OnProgress != nil {
			opt.OnProgress(msg)
		}
	}

	cacheMu.Lock()
	cached, haveCache := cache[domain]
	cacheMu.Unlock()
	if haveCache {
		report(fmt.Sprintf("cache %s:%d…", cached.Host, cached.Port))
		if err := probeAuth(ctx, cached, user, password, opt.ProbeTimeout); err == nil {
			return cached, nil
		}
	}

	cands := buildCandidates(ctx, domain)
	if len(cands) == 0 {
		return Result{}, fmt.Errorf("no candidates for %s", domain)
	}
	report(fmt.Sprintf("%d candidatos para %s…", len(cands), domain))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type outcome struct {
		r   Result
		err error
	}
	results := make(chan outcome, len(cands))
	jobs := make(chan candidate, len(cands))
	for _, c := range cands {
		jobs <- c
	}
	close(jobs)

	var wg sync.WaitGroup
	workers := opt.Workers
	if workers > len(cands) {
		workers = len(cands)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				if ctx.Err() != nil {
					return
				}
				r := Result{Host: c.Host, Port: c.Port, Encryption: c.Encryption}
				report(fmt.Sprintf("testando %s:%d (%s)…", r.Host, r.Port, r.Encryption))
				err := probeAuth(ctx, r, user, password, opt.ProbeTimeout)
				if err == nil {
					select {
					case results <- outcome{r: r}:
						cancel()
					default:
					}
					return
				}
				select {
				case results <- outcome{err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var lastErr error
	for o := range results {
		if o.err == nil {
			cacheMu.Lock()
			cache[domain] = o.r
			cacheMu.Unlock()
			report(fmt.Sprintf("ok %s:%d", o.r.Host, o.r.Port))
			return o.r, nil
		}
		lastErr = o.err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no smtp candidate worked")
	}
	return Result{}, fmt.Errorf("discover %s: %w", domain, lastErr)
}

func buildCandidates(ctx context.Context, domain string) []candidate {
	var out []candidate
	seen := map[string]struct{}{}
	add := func(host string, port int, enc string) {
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
		if host == "" {
			return
		}
		key := fmt.Sprintf("%s|%d|%s", host, port, enc)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, candidate{Host: host, Port: port, Encryption: enc})
	}
	addPair := func(host string) {
		add(host, 587, "starttls")
		add(host, 465, "ssl")
	}

	if r, ok := knownProviders[domain]; ok {
		add(r.Host, r.Port, r.Encryption)
		if r.Port == 587 {
			add(r.Host, 465, "ssl")
		}
		return out // known provider: only those hosts
	}

	mxHosts := lookupMXHosts(ctx, domain)

	// MX fingerprint → real submission host (Locaweb email-ssl, Google, M365, …). Prefer these first.
	for _, h := range submissionHostsFromMX(mxHosts) {
		addPair(h)
	}

	// Custom domain submission names (often CNAME to provider; may fail TLS SNI — still try).
	addPair("smtp." + domain)
	addPair("mail." + domain)
	addPair(domain)

	// Do not probe inbound MX as AUTH targets (timeouts / no submission).
	for _, h := range mxHosts {
		if isInboundMXHost(h) {
			continue
		}
		addPair(h)
	}

	if len(out) > 14 {
		out = out[:14]
	}
	return out
}

func lookupMXHosts(ctx context.Context, domain string) []string {
	dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	mxs, err := net.DefaultResolver.LookupMX(dctx, domain)
	if err != nil || len(mxs) == 0 {
		return nil
	}
	var hosts []string
	for i, mx := range mxs {
		if i >= 5 {
			break
		}
		h := strings.TrimSuffix(strings.ToLower(mx.Host), ".")
		if h != "" && h != "." {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func probeAuth(ctx context.Context, r Result, user, password string, timeout time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)

	// Fast fail: DNS + TCP before SMTP handshake.
	dialer := &net.Dialer{Timeout: timeout}
	if _, err := net.DefaultResolver.LookupHost(dctx, r.Host); err != nil {
		return err
	}

	enc := strings.ToLower(r.Encryption)
	useImplicit := enc == "ssl" || (enc == "tls" && r.Port != 587)

	var (
		conn net.Conn
		err  error
	)
	if useImplicit {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config: &tls.Config{
				ServerName:         r.Host,
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: false,
			},
		}
		conn, err = tlsDialer.DialContext(dctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(dctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, ok := dctx.Deadline()
	if ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, r.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if !useImplicit {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: r.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		}
	}

	// Try full email only (most submission servers want that). Local-part as one quick retry.
	users := []string{user}
	if at := strings.LastIndex(user, "@"); at > 0 {
		if local := user[:at]; local != "" {
			users = append(users, local)
		}
	}
	var last error
	for i, u := range users {
		if i > 0 {
			// AUTH failure usually kills the session — need a fresh connection for local-part.
			_ = client.Close()
			return probeAuthUser(dctx, r, u, password, dialer, useImplicit, deadline)
		}
		auth := smtp.PlainAuth("", u, password, r.Host)
		if err := client.Auth(auth); err != nil {
			last = fmt.Errorf("auth: %w", err)
			continue
		}
		return nil
	}
	return last
}

func probeAuthUser(ctx context.Context, r Result, user, password string, dialer *net.Dialer, useImplicit bool, deadline time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)
	var (
		conn net.Conn
		err  error
	)
	if useImplicit {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    &tls.Config{ServerName: r.Host, MinVersion: tls.VersionTLS12},
		}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if !deadline.IsZero() {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, r.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if !useImplicit {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: r.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	if err := client.Auth(smtp.PlainAuth("", user, password, r.Host)); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	return nil
}

// ClearCache is for tests.
func ClearCache() {
	cacheMu.Lock()
	cache = map[string]Result{}
	cacheMu.Unlock()
}
