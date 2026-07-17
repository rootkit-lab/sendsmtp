package imapdiscover

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/client"
)

// Result is a working IMAP endpoint.
type Result struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Encryption string `json:"encryption"` // ssl | starttls
}

type Options struct {
	OnProgress   func(msg string)
	ProbeTimeout time.Duration
	Workers      int
	// HintHost is usually the SMTP host (mail.domain) — tried first.
	HintHost string
}

var known = map[string]Result{
	"gmail.com":      {Host: "imap.gmail.com", Port: 993, Encryption: "ssl"},
	"googlemail.com": {Host: "imap.gmail.com", Port: 993, Encryption: "ssl"},
	"outlook.com":    {Host: "outlook.office365.com", Port: 993, Encryption: "ssl"},
	"hotmail.com":    {Host: "outlook.office365.com", Port: 993, Encryption: "ssl"},
	"live.com":       {Host: "outlook.office365.com", Port: 993, Encryption: "ssl"},
	"msn.com":        {Host: "outlook.office365.com", Port: 993, Encryption: "ssl"},
	"office365.com":  {Host: "outlook.office365.com", Port: 993, Encryption: "ssl"},
	"yahoo.com":      {Host: "imap.mail.yahoo.com", Port: 993, Encryption: "ssl"},
	"yahoo.com.br":   {Host: "imap.mail.yahoo.com", Port: 993, Encryption: "ssl"},
	"icloud.com":     {Host: "imap.mail.me.com", Port: 993, Encryption: "ssl"},
	"me.com":         {Host: "imap.mail.me.com", Port: 993, Encryption: "ssl"},
	"zoho.com":       {Host: "imap.zoho.com", Port: 993, Encryption: "ssl"},
}

type candidate struct {
	Host, Encryption string
	Port             int
}

// Discover finds IMAP that accepts LOGIN with user/password.
func Discover(ctx context.Context, domain, user, password string, opt Options) (Result, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	user = strings.TrimSpace(user)
	password = strings.TrimSpace(password)
	if domain == "" || user == "" || password == "" {
		return Result{}, fmt.Errorf("missing domain/credentials")
	}
	if opt.ProbeTimeout <= 0 {
		opt.ProbeTimeout = 3 * time.Second
	}
	if opt.Workers <= 0 {
		opt.Workers = 3
	}
	report := func(m string) {
		if opt.OnProgress != nil {
			opt.OnProgress(m)
		}
	}

	cands := buildCandidates(domain, opt.HintHost)
	report(fmt.Sprintf("IMAP %d candidatos…", len(cands)))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type out struct {
		r   Result
		err error
	}
	ch := make(chan out, len(cands))
	jobs := make(chan candidate, len(cands))
	for _, c := range cands {
		jobs <- c
	}
	close(jobs)

	var wg sync.WaitGroup
	n := opt.Workers
	if n > len(cands) {
		n = len(cands)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				if ctx.Err() != nil {
					return
				}
				r := Result{Host: c.Host, Port: c.Port, Encryption: c.Encryption}
				report(fmt.Sprintf("IMAP %s:%d…", r.Host, r.Port))
				err := probe(ctx, r, user, password, opt.ProbeTimeout)
				if err == nil {
					select {
					case ch <- out{r: r}:
						cancel()
					default:
					}
					return
				}
				select {
				case ch <- out{err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	var last error
	for o := range ch {
		if o.err == nil {
			report(fmt.Sprintf("IMAP ok %s:%d", o.r.Host, o.r.Port))
			return o.r, nil
		}
		last = o.err
	}
	if last == nil {
		last = fmt.Errorf("no imap candidate")
	}
	return Result{}, fmt.Errorf("imap discover %s: %w", domain, last)
}

func buildCandidates(domain, hint string) []candidate {
	var out []candidate
	seen := map[string]struct{}{}
	add := func(host string, port int, enc string) {
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
		if host == "" {
			return
		}
		key := fmt.Sprintf("%s|%d", host, port)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, candidate{Host: host, Port: port, Encryption: enc})
	}
	addPair := func(host string) {
		add(host, 993, "ssl")
		add(host, 143, "starttls")
	}

	if r, ok := known[domain]; ok {
		add(r.Host, r.Port, r.Encryption)
		return out
	}
	if hint != "" {
		// Locaweb SMTP host → same IMAP endpoint
		h := strings.ToLower(hint)
		if strings.Contains(h, "email-ssl") || strings.Contains(h, "locaweb") {
			addPair("email-ssl.com.br")
		}
		addPair(hint)
	}
	addPair("imap." + domain)
	addPair("mail." + domain)
	addPair(domain)
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func probe(ctx context.Context, r Result, user, password string, timeout time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)
	if _, err := net.DefaultResolver.LookupHost(dctx, r.Host); err != nil {
		return err
	}

	dialer := &net.Dialer{Timeout: timeout}
	var (
		c   *client.Client
		err error
	)
	done := make(chan error, 1)
	go func() {
		if strings.ToLower(r.Encryption) == "ssl" || r.Port == 993 {
			c, err = client.DialWithDialerTLS(dialer, addr, &tls.Config{ServerName: r.Host, MinVersion: tls.VersionTLS12})
		} else {
			c, err = client.DialWithDialer(dialer, addr)
			if err == nil {
				if err = c.StartTLS(&tls.Config{ServerName: r.Host, MinVersion: tls.VersionTLS12}); err != nil {
					_ = c.Logout()
					done <- err
					return
				}
			}
		}
		if err != nil {
			done <- err
			return
		}
		if err := c.Login(user, password); err != nil {
			_ = c.Logout()
			done <- err
			return
		}
		_ = c.Logout()
		done <- nil
	}()

	select {
	case <-dctx.Done():
		return dctx.Err()
	case err := <-done:
		return err
	}
}
