package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wiz/sendsmtp/internal/config"
	"github.com/wiz/sendsmtp/internal/emailvalid"
	"github.com/wiz/sendsmtp/internal/imapextract"
	"github.com/wiz/sendsmtp/internal/inboxcheck"
	"github.com/wiz/sendsmtp/internal/mailer"
	"github.com/wiz/sendsmtp/internal/parser"
	"github.com/wiz/sendsmtp/internal/smtpdiscover"
	"github.com/wiz/sendsmtp/internal/store"
	"github.com/wiz/sendsmtp/internal/worker"
)

type ImportResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"` // duplicates already in DB (or input dupes when not validating)
	Invalid  int `json:"invalid"` // failed validation (syntax/DNS)
	Total    int `json:"total"`
}

type AnalyzeAllResult struct {
	Total   int                                    `json:"total"`
	OK      int                                    `json:"ok"`
	Failed  int                                    `json:"failed"`
	Batches int                                    `json:"batches"`
	Results map[string]inboxcheck.PlacementSummary `json:"results"` // smtp id → summary
	Errors  map[string]string                      `json:"errors"`  // smtp id → error
}

// JobProgress is emitted on "job:progress" for UI progress bars.
type JobProgress struct {
	Job     string  `json:"job"` // emails-import | smtps-import | analyze | analyze-all | extract-contacts
	Phase   string  `json:"phase"`
	Current int     `json:"current"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
	Message string  `json:"message"`
	SMTPId  int64   `json:"smtp_id,omitempty"`
	Done    bool    `json:"done"`
}

func (e *Engine) emitJob(p JobProgress) {
	if e == nil || e.Emit == nil {
		return
	}
	if p.Total > 0 && p.Percent == 0 {
		p.Percent = float64(p.Current) / float64(p.Total) * 100
	}
	if p.Percent < 0 {
		p.Percent = 0
	}
	if p.Percent > 100 {
		p.Percent = 100
	}
	e.Emit("job:progress", p)
}

type Stats struct {
	Status      store.StatusCounts `json:"status"`
	Campaign    store.Campaign     `json:"campaign"`
	TopErrors   []string           `json:"top_errors"`
	Rate        float64            `json:"rate"`
	RateHistory []worker.RatePoint `json:"rate_history"`
	SMTPStats   []store.SMTPStat   `json:"smtp_stats"`
	Running     bool               `json:"running"`
}

// Engine is the shared core used by Wails UI and CLI.
type Engine struct {
	ConfigPath string
	Cfg        config.Config
	Store      *store.Store
	Runner     *worker.Runner
	Emit       worker.EventEmitter
}

func New(configPath string, emit worker.EventEmitter) (*Engine, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Database), 0o755); err != nil {
		return nil, err
	}
	st, err := store.Open(cfg.Database)
	if err != nil {
		return nil, err
	}
	if _, err := st.ReopenOrphans(); err != nil {
		_ = st.Close()
		return nil, err
	}
	e := &Engine{
		ConfigPath: configPath,
		Cfg:        cfg,
		Store:      st,
		Emit:       emit,
	}
	e.Runner = worker.New(st, cfg, emit)
	return e, nil
}

func (e *Engine) Close() error {
	if e.Runner != nil && e.Runner.IsRunning() {
		_ = e.Runner.Pause()
	}
	if e.Store != nil {
		return e.Store.Close()
	}
	return nil
}

func (e *Engine) ImportSmtpsText(raw string) (ImportResult, error) {
	e.emitJob(JobProgress{Job: "smtps-import", Phase: "parse", Percent: 5, Message: "Parseando SMTPs…"})

	var list []store.SMTP
	var invalid int

	if parser.LooksLikeGoscan(raw) {
		accs, err := parser.ParseGoscanSMTPs(raw)
		if err != nil {
			e.emitJob(JobProgress{Job: "smtps-import", Phase: "error", Percent: 100, Done: true, Message: err.Error()})
			return ImportResult{}, err
		}
		for _, a := range accs {
			list = append(list, store.SMTP{
				Domain:       a.Domain,
				AccountLabel: a.AccountLabel,
				Host:         a.Host,
				Port:         a.Port,
				Encryption:   a.Encryption,
				FromAddr:     a.From,
				User:         a.User,
				Password:     a.Password,
			})
		}
	} else {
		creds := parser.ParseEmailPasswordLines(raw)
		if len(creds) == 0 {
			msg := "nenhum SMTP encontrado — use goscan ou email;senha"
			e.emitJob(JobProgress{Job: "smtps-import", Phase: "error", Percent: 100, Done: true, Message: msg})
			return ImportResult{}, fmt.Errorf("%s", msg)
		}
		e.emitJob(JobProgress{
			Job: "smtps-import", Phase: "discover", Current: 0, Total: len(creds), Percent: 8,
			Message: fmt.Sprintf("Descobrindo SMTP para %d conta(s)…", len(creds)),
		})
		// Hard cap — discovery must not freeze the UI for minutes.
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		for i, a := range creds {
			e.emitJob(JobProgress{
				Job: "smtps-import", Phase: "discover", Current: i, Total: len(creds),
				Percent: 8 + float64(i)/float64(max(len(creds), 1))*55,
				Message: fmt.Sprintf("Discover %s…", a.User),
			})
			idx := i
			res, err := smtpdiscover.Discover(ctx, a.Domain, a.User, a.Password, smtpdiscover.Options{
				ProbeTimeout: 4 * time.Second,
				Workers:      4,
				OnProgress: func(msg string) {
					e.emitJob(JobProgress{
						Job: "smtps-import", Phase: "discover", Current: idx, Total: len(creds),
						Percent: 8 + (float64(idx)+0.5)/float64(max(len(creds), 1))*55,
						Message: msg,
					})
				},
			})
			if err != nil {
				invalid++
				e.emitJob(JobProgress{
					Job: "smtps-import", Phase: "discover", Current: i + 1, Total: len(creds),
					Percent: 8 + float64(i+1)/float64(max(len(creds), 1))*55,
					Message: fmt.Sprintf("falhou %s: %v", a.User, err),
				})
				continue
			}
			list = append(list, store.SMTP{
				Domain:       a.Domain,
				AccountLabel: a.AccountLabel,
				Host:         res.Host,
				Port:         res.Port,
				Encryption:   res.Encryption,
				FromAddr:     a.From,
				User:         a.User,
				Password:     a.Password,
			})
		}
		if len(list) == 0 {
			msg := fmt.Sprintf("discovery falhou para todas as %d conta(s)", len(creds))
			e.emitJob(JobProgress{Job: "smtps-import", Phase: "error", Percent: 100, Done: true, Message: msg})
			return ImportResult{Invalid: invalid, Total: len(creds)}, fmt.Errorf("%s", msg)
		}
	}

	e.emitJob(JobProgress{Job: "smtps-import", Phase: "save", Current: 0, Total: len(list), Percent: 70, Message: "Salvando SMTPs…"})
	ins, upd, err := e.Store.UpsertSMTPsProgress(list, func(done, total int) {
		pct := 70 + float64(done)/float64(max(total, 1))*25
		e.emitJob(JobProgress{
			Job: "smtps-import", Phase: "save", Current: done, Total: total, Percent: pct,
			Message: fmt.Sprintf("Salvando SMTPs %d/%d…", done, total),
		})
	})
	if err != nil {
		e.emitJob(JobProgress{Job: "smtps-import", Phase: "error", Percent: 100, Done: true, Message: err.Error()})
		return ImportResult{}, err
	}
	e.emitJob(JobProgress{
		Job: "smtps-import", Phase: "done", Current: len(list), Total: len(list) + invalid, Percent: 100, Done: true,
		Message: fmt.Sprintf("%d novos, %d atualizados%s", ins, upd, func() string {
			if invalid > 0 {
				return fmt.Sprintf(", %d sem discovery", invalid)
			}
			return ""
		}()),
	})
	return ImportResult{Inserted: ins, Updated: upd, Invalid: invalid, Total: len(list) + invalid}, nil
}

func (e *Engine) ImportEmailsText(raw string, validate bool) (ImportResult, error) {
	return e.importEmailsLines(parser.ParseLines(raw), validate)
}

// ImportEmailsFile loads a large list from disk (avoids UI/IPC paste limits).
func (e *Engine) ImportEmailsFile(path string, validate bool) (ImportResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ImportResult{}, fmt.Errorf("caminho vazio")
	}
	f, err := os.Open(path)
	if err != nil {
		return ImportResult{}, err
	}
	defer f.Close()
	lines := parser.ParseLinesReader(f)
	e.emitJob(JobProgress{
		Job: "emails-import", Phase: "start", Current: 0, Total: len(lines), Percent: 1,
		Message: fmt.Sprintf("Arquivo: %d linhas…", len(lines)),
	})
	return e.importEmailsLines(lines, validate)
}

func (e *Engine) importEmailsLines(lines []string, validate bool) (ImportResult, error) {
	toImport := lines
	var invalid, inputDupes int
	totalLines := len(lines)
	e.emitJob(JobProgress{
		Job: "emails-import", Phase: "start", Current: 0, Total: totalLines, Percent: 1,
		Message: fmt.Sprintf("Preparando %d linhas…", totalLines),
	})

	if validate {
		e.emitJob(JobProgress{
			Job: "emails-import", Phase: "validate", Current: 0, Total: totalLines, Percent: 5,
			Message: "Validando sintaxe + MX (domínios reais)…",
		})
		// Scale timeout with list size (unique domains unknown yet — use line count as upper bound).
		deadline := 15 * time.Minute
		if totalLines > 50000 {
			deadline = 45 * time.Minute
		} else if totalLines > 20000 {
			deadline = 30 * time.Minute
		}
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()
		fr := emailvalid.Filter(ctx, lines, emailvalid.Options{
			Workers:       8,
			Timeout:       3 * time.Second,
			Pace:          20 * time.Millisecond,
			ProgressEvery: 300 * time.Millisecond,
			RequireMX:     true,
			OnProgress: func(done, total int) {
				pct := 5 + float64(done)/float64(max(total, 1))*70
				e.emitJob(JobProgress{
					Job: "emails-import", Phase: "validate", Current: done, Total: total, Percent: pct,
					Message: fmt.Sprintf("DNS MX %d/%d domínios…", done, total),
				})
			},
		})
		toImport = fr.Valid
		invalid = len(fr.Invalid)
		inputDupes = fr.Duplicate
	} else {
		seen := make(map[string]struct{}, len(lines))
		unique := make([]string, 0, len(lines))
		for _, line := range lines {
			addr := emailvalid.Normalize(line)
			if addr == "" || !strings.Contains(addr, "@") || !emailvalid.SyntaxOK(addr) {
				invalid++
				continue
			}
			if _, ok := seen[addr]; ok {
				inputDupes++
				continue
			}
			seen[addr] = struct{}{}
			unique = append(unique, addr)
		}
		toImport = unique
	}

	e.emitJob(JobProgress{
		Job: "emails-import", Phase: "import", Current: 0, Total: len(toImport), Percent: 80,
		Message: fmt.Sprintf("Importando %d únicos…", len(toImport)),
	})
	ins, skip, err := e.Store.ImportEmailsProgress(toImport, func(done, total int) {
		pct := 80 + float64(done)/float64(max(total, 1))*19
		e.emitJob(JobProgress{
			Job: "emails-import", Phase: "import", Current: done, Total: total, Percent: pct,
			Message: fmt.Sprintf("Gravando %d/%d…", done, total),
		})
	})
	if err != nil {
		e.emitJob(JobProgress{Job: "emails-import", Phase: "error", Percent: 100, Done: true, Message: err.Error()})
		return ImportResult{}, err
	}
	e.emitJob(JobProgress{
		Job: "emails-import", Phase: "done", Current: ins, Total: totalLines, Percent: 100, Done: true,
		Message: fmt.Sprintf("%d inseridos, %d ignorados, %d inválidos", ins, skip+inputDupes, invalid),
	})
	return ImportResult{
		Inserted: ins,
		Skipped:  skip + inputDupes,
		Invalid:  invalid,
		Total:    totalLines,
	}, nil
}

func (e *Engine) ImportSubjectsText(raw string) (ImportResult, error) {
	lines := parser.ParseLines(raw)
	if err := e.Store.ReplaceSubjects(lines); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Inserted: len(lines), Total: len(lines)}, nil
}

func (e *Engine) ImportLinksText(raw string) (ImportResult, error) {
	lines := parser.ParseLines(raw)
	if err := e.Store.ReplaceLinks(lines); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Inserted: len(lines), Total: len(lines)}, nil
}

func (e *Engine) SetHTML(html string) error {
	return e.Store.SetCampaignHTML(html, e.Cfg.FromName)
}

func (e *Engine) ImportAll() (map[string]ImportResult, error) {
	out := map[string]ImportResult{}
	read := func(path string) (string, error) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if raw, err := read(e.Cfg.Paths.Smtps); err == nil {
		r, err := e.ImportSmtpsText(raw)
		if err != nil {
			return out, fmt.Errorf("smtps: %w", err)
		}
		out["smtps"] = r
	}
	if raw, err := read(e.Cfg.Paths.Emails); err == nil {
		r, err := e.ImportEmailsText(raw, false)
		if err != nil {
			return out, fmt.Errorf("emails: %w", err)
		}
		out["emails"] = r
	}
	if raw, err := read(e.Cfg.Paths.Subjects); err == nil {
		r, err := e.ImportSubjectsText(raw)
		if err != nil {
			return out, fmt.Errorf("subjects: %w", err)
		}
		out["subjects"] = r
	}
	if raw, err := read(e.Cfg.Paths.Links); err == nil {
		r, err := e.ImportLinksText(raw)
		if err != nil {
			return out, fmt.Errorf("links: %w", err)
		}
		out["links"] = r
	}
	if raw, err := read(e.Cfg.Paths.HTML); err == nil {
		if err := e.SetHTML(raw); err != nil {
			return out, fmt.Errorf("html: %w", err)
		}
		out["html"] = ImportResult{Inserted: 1, Total: 1}
	}
	return out, nil
}

func (e *Engine) ListSmtps() ([]store.SMTP, error) {
	list, err := e.Store.ListSMTPs()
	if err != nil {
		return nil, err
	}
	// hide passwords in UI listings by default — still needed for send from store.GetSMTP
	for i := range list {
		list[i].Password = ""
	}
	return list, nil
}

// ExtractMailboxResult is IMAP scrape + optional import counts.
type ExtractMailboxResult struct {
	IMAPHost        string                   `json:"imap_host"`
	IMAPPort        int                      `json:"imap_port"`
	IMAPEnc         string                   `json:"imap_encryption"`
	Messages        int                      `json:"messages_scanned"`
	Contacts        []string                 `json:"contacts"` // preview (full list is on disk + email queue)
	Credentials     []imapextract.Credential `json:"credentials"`
	ContactsFile    string                   `json:"contacts_file"`
	CredsFile       string                   `json:"creds_file"`
	ContactCount    int                      `json:"contact_count"`
	ImportedEmails  int                      `json:"imported_emails"`
	SkippedEmails   int                      `json:"skipped_emails"`
	ImportedSmtps   int                      `json:"imported_smtps"`
	UpdatedSmtps    int                      `json:"updated_smtps"`
}

// ExtractSmtpContacts discovers IMAP for one SMTP, scrapes contacts/passwords from mail,
// writes data/extracted/*, and always imports contacts into the Emails queue.
func (e *Engine) ExtractSmtpContacts(id int64) (ExtractMailboxResult, error) {
	acc, err := e.Store.GetSMTP(id)
	if err != nil {
		return ExtractMailboxResult{}, err
	}
	if acc.Password == "" {
		return ExtractMailboxResult{}, fmt.Errorf("SMTP #%d sem senha", id)
	}
	domain := strings.ToLower(strings.TrimSpace(acc.Domain))
	user := strings.TrimSpace(acc.User)
	if user == "" {
		user = strings.TrimSpace(acc.FromAddr)
	}
	if domain == "" {
		if at := strings.LastIndex(user, "@"); at > 0 {
			domain = strings.ToLower(user[at+1:])
		}
	}
	if domain == "" {
		return ExtractMailboxResult{}, fmt.Errorf("SMTP #%d sem domínio", id)
	}

	e.emitJob(JobProgress{
		Job: "extract-contacts", Phase: "start", SMTPId: id, Percent: 2,
		Message: fmt.Sprintf("Extraindo contatos SMTP #%d…", id),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	raw, err := imapextract.Extract(ctx, domain, user, acc.Password, acc.Host, imapextract.Options{
		MaxMessages:  150,
		ProbeTimeout: 3 * time.Second,
		OnProgress: func(msg string) {
			e.emitJob(JobProgress{
				Job: "extract-contacts", Phase: "imap", SMTPId: id, Percent: 20,
				Message: msg,
			})
		},
	})
	if err != nil {
		e.emitJob(JobProgress{Job: "extract-contacts", Phase: "error", SMTPId: id, Percent: 100, Done: true, Message: err.Error()})
		return ExtractMailboxResult{}, err
	}

	outDir := filepath.Join("data", "extracted")
	_ = os.MkdirAll(outDir, 0o755)
	contactsPath := filepath.Join(outDir, fmt.Sprintf("contacts-smtp-%d.txt", id))
	credsPath := filepath.Join(outDir, fmt.Sprintf("creds-smtp-%d.txt", id))

	sortStrings(raw.Contacts)
	if err := os.WriteFile(contactsPath, []byte(strings.Join(raw.Contacts, "\n")+"\n"), 0o600); err != nil {
		return ExtractMailboxResult{}, err
	}
	var credLines []string
	for _, c := range raw.Credentials {
		credLines = append(credLines, c.Email+";"+c.Password)
	}
	sortStrings(credLines)
	if err := os.WriteFile(credsPath, []byte(strings.Join(credLines, "\n")+"\n"), 0o600); err != nil {
		return ExtractMailboxResult{}, err
	}

	// Merge contacts + credential emails into the Emails queue (pending).
	toQueue := make([]string, 0, len(raw.Contacts)+len(raw.Credentials))
	toQueue = append(toQueue, raw.Contacts...)
	for _, c := range raw.Credentials {
		toQueue = append(toQueue, c.Email)
	}

	e.emitJob(JobProgress{
		Job: "extract-contacts", Phase: "import", SMTPId: id, Percent: 75,
		Message: fmt.Sprintf("Adicionando %d contatos à lista de Emails…", len(toQueue)),
	})
	insEmails, skipEmails, err := e.Store.ImportEmailsProgress(toQueue, func(done, total int) {
		pct := 75 + float64(done)/float64(max(total, 1))*8
		e.emitJob(JobProgress{
			Job: "extract-contacts", Phase: "import", SMTPId: id, Current: done, Total: total, Percent: pct,
			Message: fmt.Sprintf("Lista de Emails %d/%d…", done, total),
		})
	})
	if err != nil {
		e.emitJob(JobProgress{Job: "extract-contacts", Phase: "error", SMTPId: id, Percent: 100, Done: true, Message: err.Error()})
		return ExtractMailboxResult{}, fmt.Errorf("importar emails: %w", err)
	}

	insSmtps, updSmtps := 0, 0
	if len(credLines) > 0 {
		e.emitJob(JobProgress{
			Job: "extract-contacts", Phase: "import-smtps", SMTPId: id, Percent: 85,
			Message: fmt.Sprintf("Importando %d credenciais SMTP…", len(credLines)),
		})
		res, err := e.ImportSmtpsText(strings.Join(credLines, "\n"))
		if err == nil {
			insSmtps, updSmtps = res.Inserted, res.Updated
		}
	}

	preview := raw.Contacts
	if len(preview) > 100 {
		preview = preview[:100]
	}
	out := ExtractMailboxResult{
		IMAPHost:       raw.IMAPHost,
		IMAPPort:       raw.IMAPPort,
		IMAPEnc:        raw.IMAPEnc,
		Messages:       raw.Messages,
		Contacts:       preview,
		Credentials:    raw.Credentials,
		ContactsFile:   contactsPath,
		CredsFile:      credsPath,
		ContactCount:   len(raw.Contacts),
		ImportedEmails: insEmails,
		SkippedEmails:  skipEmails,
		ImportedSmtps:  insSmtps,
		UpdatedSmtps:   updSmtps,
	}
	e.emitJob(JobProgress{
		Job: "extract-contacts", Phase: "done", SMTPId: id, Percent: 100, Done: true,
		Message: fmt.Sprintf("%d contatos → lista (+%d novos, %d já existiam) · %d senhas · IMAP %s:%d",
			out.ContactCount, out.ImportedEmails, out.SkippedEmails, len(out.Credentials), out.IMAPHost, out.IMAPPort),
	})
	return out, nil
}

// ExtractAllSmtpContacts runs ExtractSmtpContacts for every stored SMTP.
func (e *Engine) ExtractAllSmtpContacts() (map[string]ExtractMailboxResult, error) {
	list, err := e.Store.ListSMTPs()
	if err != nil {
		return nil, err
	}
	out := map[string]ExtractMailboxResult{}
	for i, s := range list {
		e.emitJob(JobProgress{
			Job: "extract-contacts", Phase: "batch", Current: i, Total: len(list),
			Percent: float64(i) / float64(max(len(list), 1)) * 100,
			Message: fmt.Sprintf("Extrair %d/%d…", i+1, len(list)),
			SMTPId:  s.ID,
		})
		r, err := e.ExtractSmtpContacts(s.ID)
		key := fmt.Sprintf("%d", s.ID)
		if err != nil {
			out[key] = ExtractMailboxResult{}
			continue
		}
		out[key] = r
	}
	e.emitJob(JobProgress{
		Job: "extract-contacts", Phase: "done", Percent: 100, Done: true,
		Message: fmt.Sprintf("extração em %d SMTP(s)", len(list)),
	})
	return out, nil
}

func sortStrings(s []string) {
	sort.Strings(s)
}

func (e *Engine) SetSmtpActive(id int64, active bool) error {
	status := "disabled"
	if active {
		status = "active"
	}
	return e.Store.SetSMTPStatus(id, status)
}

func (e *Engine) TestSmtp(id int64, to string) error {
	acc, err := e.Store.GetSMTP(id)
	if err != nil {
		return err
	}
	if to == "" {
		to = acc.FromAddr
	}
	msg := mailer.Message{
		FromName: e.Cfg.FromName,
		From:     acc.FromAddr,
		To:       to,
		Subject:  "SendSMTP test",
		HTML:     "<p>SendSMTP connection test OK</p>",
	}
	dialTO := time.Duration(e.Cfg.DialTimeoutSec) * time.Second
	sendTO := time.Duration(e.Cfg.SendTimeoutSec) * time.Second
	return mailer.Send(mailer.FromStore(acc.Host, acc.Port, acc.Encryption, acc.User, acc.Password, acc.FromAddr), msg, dialTO, sendTO)
}

func (e *Engine) inboxOpts() (waitSec, timeoutSec int, opt inboxcheck.Options) {
	waitSec = e.Cfg.InboxCheck.WaitSec
	if waitSec <= 0 {
		waitSec = 60
	}
	timeoutSec = e.Cfg.InboxCheck.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 240
	}
	opt = inboxcheck.Options{
		Headless:   e.Cfg.InboxCheck.Headless,
		WaitSec:    waitSec,
		TimeoutSec: timeoutSec,
	}
	return waitSec, timeoutSec, opt
}

func (e *Engine) createMailreachTest(ctx context.Context, opt inboxcheck.Options) (*inboxcheck.Test, *inboxcheck.Client, error) {
	client := inboxcheck.NewClient()
	test, err := client.CreateTest(ctx)
	if err != nil {
		test, err = inboxcheck.CreateTestWithPlaywright(ctx, opt)
		if err != nil {
			return nil, nil, fmt.Errorf("mailreach create failed: %w", err)
		}
	}
	if len(inboxcheck.SeedEmails(test)) == 0 {
		return nil, nil, fmt.Errorf("mailreach: nenhuma seed no teste")
	}
	return test, client, nil
}

func (e *Engine) campaignHTML(fullID string) (html, subject string) {
	html, _ = e.Store.GetHTML()
	if strings.TrimSpace(html) == "" {
		html = "<p>SendSMTP MailReach spam test</p>"
	}
	html = inboxcheck.EmbedCode(html, fullID)
	subject = "SendSMTP check " + fullID
	return html, subject
}

func (e *Engine) sendSeeds(acc store.SMTP, html, subject string, seeds []string, onProgress func(done, total int)) (ok, fail int) {
	dialTO := time.Duration(e.Cfg.DialTimeoutSec) * time.Second
	sendTO := time.Duration(e.Cfg.SendTimeoutSec) * time.Second
	total := len(seeds)
	for i, to := range seeds {
		body := mailer.Prepare(html, to, "https://example.com", subject, acc.FromAddr)
		msg := mailer.Message{
			FromName: e.Cfg.FromName,
			From:     acc.FromAddr,
			To:       to,
			Subject:  subject,
			HTML:     body,
		}
		if err := mailer.Send(mailer.FromStore(acc.Host, acc.Port, acc.Encryption, acc.User, acc.Password, acc.FromAddr), msg, dialTO, sendTO); err != nil {
			fail++
		} else {
			ok++
		}
		if onProgress != nil {
			onProgress(i+1, total)
		}
	}
	return ok, fail
}

func (e *Engine) waitWithProgress(ctx context.Context, wait time.Duration, job string, smtpID int64, basePct, spanPct float64) error {
	if wait <= 0 {
		return nil
	}
	deadline := time.Now().Add(wait)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	start := time.Now()
	for {
		elapsed := time.Since(start)
		if elapsed >= wait {
			return nil
		}
		frac := float64(elapsed) / float64(wait)
		e.emitJob(JobProgress{
			Job: job, Phase: "wait", SMTPId: smtpID,
			Percent: basePct + frac*spanPct,
			Message: fmt.Sprintf("Aguardando entrega… %ds restantes", int((wait-elapsed).Seconds())+1),
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil
			}
		}
	}
}

func (e *Engine) saveSpamSummary(id int64, sum inboxcheck.PlacementSummary) error {
	summary := fmt.Sprintf("%s · score=%.0f inbox=%d spam=%d other=%d missing=%d",
		sum.Label, sum.Score, sum.Inbox, sum.Spam, sum.Other, sum.Missing)
	return e.Store.UpdateSMTPSpamResult(id, sum.Score, sum.InboxRate, sum.Label, sum.PublicID, summary)
}

// AnalyzeSmtp runs the free MailReach spam test for one SMTP (all seeds).
func (e *Engine) AnalyzeSmtp(id int64) (inboxcheck.PlacementSummary, error) {
	acc, err := e.Store.GetSMTP(id)
	if err != nil {
		return inboxcheck.PlacementSummary{}, err
	}
	waitSec, timeoutSec, opt := e.inboxOpts()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec+waitSec+120)*time.Second)
	defer cancel()

	e.emitJob(JobProgress{Job: "analyze", Phase: "create", SMTPId: id, Percent: 3, Message: "Criando teste MailReach…"})
	test, client, err := e.createMailreachTest(ctx, opt)
	if err != nil {
		e.emitJob(JobProgress{Job: "analyze", Phase: "error", SMTPId: id, Percent: 100, Done: true, Message: err.Error()})
		return inboxcheck.PlacementSummary{}, err
	}
	seeds := inboxcheck.SeedEmails(test)
	html, subject := e.campaignHTML(test.PublicFullID)
	e.emitJob(JobProgress{
		Job: "analyze", Phase: "send", SMTPId: id, Current: 0, Total: len(seeds), Percent: 10,
		Message: fmt.Sprintf("Enviando para %d seeds…", len(seeds)),
	})
	sendOK, sendFails := e.sendSeeds(acc, html, subject, seeds, func(done, total int) {
		pct := 10 + float64(done)/float64(max(total, 1))*40
		e.emitJob(JobProgress{
			Job: "analyze", Phase: "send", SMTPId: id, Current: done, Total: total, Percent: pct,
			Message: fmt.Sprintf("Enviando seeds %d/%d…", done, total),
		})
	})
	if sendOK == 0 {
		err := fmt.Errorf("falha ao enviar para todas as seeds MailReach (%d)", sendFails)
		e.emitJob(JobProgress{Job: "analyze", Phase: "error", SMTPId: id, Percent: 100, Done: true, Message: err.Error()})
		return inboxcheck.PlacementSummary{}, err
	}
	if err := e.waitWithProgress(ctx, time.Duration(waitSec)*time.Second, "analyze", id, 50, 20); err != nil {
		e.emitJob(JobProgress{Job: "analyze", Phase: "error", SMTPId: id, Percent: 100, Done: true, Message: err.Error()})
		return inboxcheck.PlacementSummary{}, err
	}
	e.emitJob(JobProgress{Job: "analyze", Phase: "poll", SMTPId: id, Percent: 72, Message: "Consultando placement…"})
	final, err := client.PollUntilDone(ctx, test.PublicID, 5*time.Second, func(placed, total int) {
		pct := 72.0
		if total > 0 {
			pct = 72 + float64(placed)/float64(total)*25
		}
		e.emitJob(JobProgress{
			Job: "analyze", Phase: "poll", SMTPId: id, Current: placed, Total: total, Percent: pct,
			Message: fmt.Sprintf("Placement %d/%d caixas…", placed, total),
		})
	})
	if err != nil && final == nil {
		e.emitJob(JobProgress{Job: "analyze", Phase: "error", SMTPId: id, Percent: 100, Done: true, Message: err.Error()})
		return inboxcheck.PlacementSummary{}, err
	}
	if final == nil {
		final = test
	}
	sum := inboxcheck.SummarizeTest(final)
	if sendFails > 0 {
		sum.Summary += fmt.Sprintf(" · send_fail=%d/%d", sendFails, len(seeds))
	}
	if err := e.saveSpamSummary(id, sum); err != nil {
		e.emitJob(JobProgress{Job: "analyze", Phase: "error", SMTPId: id, Percent: 100, Done: true, Message: err.Error()})
		return sum, err
	}
	e.emitJob(JobProgress{
		Job: "analyze", Phase: "done", SMTPId: id, Percent: 100, Done: true,
		Message: fmt.Sprintf("%s · score=%.0f", sum.Label, sum.Score),
	})
	return sum, nil
}

// AnalyzeAllSmtps checks every SMTP using MailReach seed lists.
// When there are more SMTPs than seeds in one test, seeds are partitioned (1+ per SMTP)
// and extra SMTPs run in subsequent MailReach tests (batches).
func (e *Engine) AnalyzeAllSmtps() (AnalyzeAllResult, error) {
	list, err := e.Store.ListSMTPs()
	if err != nil {
		return AnalyzeAllResult{}, err
	}
	out := AnalyzeAllResult{
		Total:   len(list),
		Results: map[string]inboxcheck.PlacementSummary{},
		Errors:  map[string]string{},
	}
	if len(list) == 0 {
		e.emitJob(JobProgress{Job: "analyze-all", Phase: "done", Percent: 100, Done: true, Message: "Nenhum SMTP"})
		return out, nil
	}

	waitSec, timeoutSec, opt := e.inboxOpts()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration((timeoutSec+waitSec+180)*len(list))*time.Second)
	defer cancel()

	totalSMTPs := len(list)
	doneSMTPs := 0
	remaining := list
	e.emitJob(JobProgress{
		Job: "analyze-all", Phase: "start", Current: 0, Total: totalSMTPs, Percent: 1,
		Message: fmt.Sprintf("Check all: %d SMTPs…", totalSMTPs),
	})

	for len(remaining) > 0 {
		batchCtx, batchCancel := context.WithTimeout(ctx, time.Duration(timeoutSec+waitSec+180)*time.Second)
		e.emitJob(JobProgress{
			Job: "analyze-all", Phase: "create", Current: doneSMTPs, Total: totalSMTPs,
			Percent: float64(doneSMTPs) / float64(totalSMTPs) * 100,
			Message: fmt.Sprintf("Criando teste MailReach (lote %d)…", out.Batches+1),
		})
		test, client, err := e.createMailreachTest(batchCtx, opt)
		if err != nil {
			batchCancel()
			for _, s := range remaining {
				out.Failed++
				out.Errors[fmt.Sprintf("%d", s.ID)] = err.Error()
			}
			break
		}
		seeds := inboxcheck.SeedEmails(test)
		n := len(seeds)
		if n > len(remaining) {
			n = len(remaining)
		}
		batch := remaining[:n]
		remaining = remaining[n:]
		out.Batches++

		assign := make([][]string, len(batch))
		for i, seed := range seeds {
			assign[i%len(batch)] = append(assign[i%len(batch)], seed)
		}

		html, subject := e.campaignHTML(test.PublicFullID)
		sentBySMTP := make(map[int64]map[string]struct{}, len(batch))
		for i, acc := range batch {
			mine := assign[i]
			base := float64(doneSMTPs) / float64(totalSMTPs) * 100
			span := float64(len(batch)) / float64(totalSMTPs) * 40
			e.emitJob(JobProgress{
				Job: "analyze-all", Phase: "send", SMTPId: acc.ID, Current: doneSMTPs + i, Total: totalSMTPs,
				Percent: base + float64(i)/float64(max(len(batch), 1))*span,
				Message: fmt.Sprintf("SMTP #%d enviando %d seeds…", acc.ID, len(mine)),
			})
			ok, _ := e.sendSeeds(acc, html, subject, mine, nil)
			allow := make(map[string]struct{}, len(mine))
			for _, addr := range mine {
				allow[strings.ToLower(addr)] = struct{}{}
			}
			sentBySMTP[acc.ID] = allow
			if ok == 0 {
				out.Failed++
				out.Errors[fmt.Sprintf("%d", acc.ID)] = "falha ao enviar para seeds atribuídas"
				delete(sentBySMTP, acc.ID)
			}
		}

		if len(sentBySMTP) == 0 {
			batchCancel()
			doneSMTPs += len(batch)
			continue
		}

		baseWait := float64(doneSMTPs) / float64(totalSMTPs) * 100
		_ = e.waitWithProgress(batchCtx, time.Duration(waitSec)*time.Second, "analyze-all", 0, baseWait+40, 15)
		e.emitJob(JobProgress{
			Job: "analyze-all", Phase: "poll", Current: doneSMTPs, Total: totalSMTPs,
			Percent: baseWait + 55,
			Message: "Consultando placement do lote…",
		})
		final, err := client.PollUntilDone(batchCtx, test.PublicID, 5*time.Second, func(placed, total int) {
			pct := baseWait + 55
			if total > 0 {
				pct = baseWait + 55 + float64(placed)/float64(total)*20
			}
			e.emitJob(JobProgress{
				Job: "analyze-all", Phase: "poll", Current: doneSMTPs, Total: totalSMTPs, Percent: pct,
				Message: fmt.Sprintf("Placement lote %d/%d caixas…", placed, total),
			})
		})
		batchCancel()
		if final == nil {
			final = test
		}
		if err != nil && final == nil {
			for id := range sentBySMTP {
				out.Failed++
				out.Errors[fmt.Sprintf("%d", id)] = err.Error()
			}
			doneSMTPs += len(batch)
			continue
		}

		for _, acc := range batch {
			allow, ok := sentBySMTP[acc.ID]
			if !ok {
				continue
			}
			sum := inboxcheck.SummarizeTestFiltered(final, allow)
			sum.Summary += fmt.Sprintf(" · seeds=%d/%d batch", len(allow), len(seeds))
			if err := e.saveSpamSummary(acc.ID, sum); err != nil {
				out.Failed++
				out.Errors[fmt.Sprintf("%d", acc.ID)] = err.Error()
				continue
			}
			out.OK++
			out.Results[fmt.Sprintf("%d", acc.ID)] = sum
		}
		doneSMTPs += len(batch)
		e.emitJob(JobProgress{
			Job: "analyze-all", Phase: "batch-done", Current: doneSMTPs, Total: totalSMTPs,
			Percent: float64(doneSMTPs) / float64(totalSMTPs) * 100,
			Message: fmt.Sprintf("Lote %d ok · %d/%d SMTPs", out.Batches, doneSMTPs, totalSMTPs),
		})
	}
	e.emitJob(JobProgress{
		Job: "analyze-all", Phase: "done", Current: totalSMTPs, Total: totalSMTPs, Percent: 100, Done: true,
		Message: fmt.Sprintf("%d ok, %d falha, %d lote(s)", out.OK, out.Failed, out.Batches),
	})
	return out, nil
}

func (e *Engine) ListEmails(filter string, limit, offset int) ([]store.Email, error) {
	return e.Store.ListEmails(filter, limit, offset)
}

func (e *Engine) ListEmailsPage(filter, query string, limit, offset int) (store.EmailPage, error) {
	return e.Store.ListEmailsPage(filter, query, limit, offset)
}

func (e *Engine) ResetFailed() (int64, error) {
	return e.Store.ResetFailed()
}

func (e *Engine) DeleteAllEmails() (int64, error) {
	if e.Runner.IsRunning() {
		_ = e.Runner.Pause()
	}
	return e.Store.DeleteAllEmails()
}

func (e *Engine) ClearErrorLogs() (map[string]int64, error) {
	emails, smtps, err := e.Store.ClearErrorLogs()
	if err != nil {
		return nil, err
	}
	return map[string]int64{"emails": emails, "smtps": smtps}, nil
}

func (e *Engine) ReenableSMTPs() (int64, error) {
	return e.Store.ReenableAllSMTPs()
}

func (e *Engine) GetConfig() config.Config {
	return e.Cfg
}

func (e *Engine) SaveConfig(cfg config.Config) error {
	cfg.ApplyDefaults()
	if err := config.Save(e.ConfigPath, cfg); err != nil {
		return err
	}
	e.Cfg = cfg
	e.Runner.UpdateConfig(cfg)
	return nil
}

func (e *Engine) GetSubjects() ([]string, error) { return e.Store.ListSubjects() }
func (e *Engine) GetLinks() ([]string, error)    { return e.Store.ListLinks() }
func (e *Engine) GetHTML() (string, error)       { return e.Store.GetHTML() }

func (e *Engine) StartCampaign() error {
	html, _ := e.Store.GetHTML()
	if html == "" {
		return fmt.Errorf("HTML body is empty — set content first")
	}
	// Merge near-duplicate addresses is done at import (Normalize + UNIQUE).
	// Full-table dedupe at Start is O(n) RAM/CPU and freezes the UI with large lists.
	if _, err := e.Store.ReenableCooldownSMTPs(); err != nil {
		return err
	}
	n, err := e.Store.CountActiveSMTPs()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no active SMTPs")
	}
	st, err := e.Store.GetStatus()
	if err != nil {
		return err
	}
	if st.Pending == 0 {
		if st.Failed > 0 {
			return fmt.Errorf("no pending emails — use Reset failed first (%d failed)", st.Failed)
		}
		return fmt.Errorf("no pending emails")
	}
	e.Runner.UpdateConfig(e.Cfg)
	return e.Runner.Start()
}

func (e *Engine) PauseCampaign() error {
	return e.Runner.Pause()
}

func (e *Engine) ResumeCampaign() error {
	return e.StartCampaign()
}

func (e *Engine) GetStatus() (store.StatusCounts, error) {
	return e.Store.GetStatus()
}

func (e *Engine) GetStats() (Stats, error) {
	st, err := e.Store.GetStatus()
	if err != nil {
		return Stats{}, err
	}
	camp, err := e.Store.GetCampaign()
	if err != nil {
		return Stats{}, err
	}
	errs, _ := e.Store.TopErrors(10)
	smtpStats, _ := e.Store.SMTPStats()
	p := e.Runner.CurrentProgress()
	if e.Runner.IsRunning() {
		camp.State = "running"
	} else if camp.State == "running" {
		// Orphaned DB state after crash / workers exited — expose as paused.
		camp.State = "paused"
		_ = e.Store.SetCampaignState("paused")
	}
	return Stats{
		Status:      st,
		Campaign:    camp,
		TopErrors:   errs,
		Rate:        p.Rate,
		RateHistory: e.Runner.RateHistory(),
		SMTPStats:   smtpStats,
		Running:     e.Runner.IsRunning(),
	}, nil
}
