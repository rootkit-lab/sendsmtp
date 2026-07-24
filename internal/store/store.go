package store

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wiz/sendsmtp/internal/emailvalid"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB

	topErrMu    sync.Mutex
	topErrCache []string
	topErrAt    time.Time
}

type SMTP struct {
	ID            int64   `json:"id"`
	Domain        string  `json:"domain"`
	AccountLabel  string  `json:"account_label"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Encryption    string  `json:"encryption"`
	FromAddr      string  `json:"from_addr"`
	User          string  `json:"user"`
	Password      string  `json:"password,omitempty"`
	Status        string  `json:"status"`
	FailCount     int     `json:"fail_count"`
	LastError     string  `json:"last_error"`
	SentCount     int64   `json:"sent_count"`
	InboxScore    float64 `json:"inbox_score"`
	InboxRate     float64 `json:"inbox_rate"`
	InboxLabel    string  `json:"inbox_label"`
	SpamTestID    string  `json:"spam_test_id"`
	SpamCheckedAt string  `json:"spam_checked_at"`
	SpamSummary   string  `json:"spam_summary"`
}

type Email struct {
	ID        int64  `json:"id"`
	Address   string `json:"address"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	SMTPID    *int64 `json:"smtp_id,omitempty"`
	Error     string `json:"error"`
	Subject   string `json:"subject"`
	Link      string `json:"link"`
	UpdatedAt string `json:"updated_at"`
}

// EmailPage is a paginated slice of emails plus total matching rows.
type EmailPage struct {
	Items  []Email `json:"items"`
	Total  int64   `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type StatusCounts struct {
	Pending       int64 `json:"pending"`
	Sending       int64 `json:"sending"`
	Sent          int64 `json:"sent"`
	Failed        int64 `json:"failed"`
	SMTPsActive   int64 `json:"smtps_active"`
	SMTPsDisabled int64 `json:"smtps_disabled"`
}

type Campaign struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	HTML      string `json:"html"`
	FromName  string `json:"from_name"`
	StartedAt string `json:"started_at"`
	UpdatedAt string `json:"updated_at"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS smtps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  domain TEXT NOT NULL DEFAULT '',
  account_label TEXT NOT NULL DEFAULT '',
  host TEXT NOT NULL,
  port INTEGER NOT NULL,
  encryption TEXT NOT NULL DEFAULT 'tls',
  from_addr TEXT NOT NULL,
  user TEXT NOT NULL,
  password TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  fail_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  sent_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(host, user)
);

CREATE TABLE IF NOT EXISTS emails (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  address TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  smtp_id INTEGER,
  error TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL DEFAULT '',
  link TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_emails_status ON emails(status);
CREATE INDEX IF NOT EXISTS idx_emails_status_id ON emails(status, id);

CREATE TABLE IF NOT EXISTS email_counts (
  status TEXT PRIMARY KEY,
  n INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS subjects (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  text TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  url TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS campaigns (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  state TEXT NOT NULL DEFAULT 'idle',
  html TEXT NOT NULL DEFAULT '',
  from_name TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO campaigns (id, state) VALUES (1, 'idle');

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}
	for _, q := range []string{
		`ALTER TABLE smtps ADD COLUMN inbox_score REAL NOT NULL DEFAULT -1`,
		`ALTER TABLE smtps ADD COLUMN inbox_rate REAL NOT NULL DEFAULT -1`,
		`ALTER TABLE smtps ADD COLUMN inbox_label TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE smtps ADD COLUMN spam_test_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE smtps ADD COLUMN spam_checked_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE smtps ADD COLUMN spam_summary TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_emails_status_id ON emails(status, id)`,
		`CREATE TABLE IF NOT EXISTS email_counts (status TEXT PRIMARY KEY, n INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS servers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  host TEXT NOT NULL,
  ssh_port INTEGER NOT NULL DEFAULT 22,
  ssh_user TEXT NOT NULL DEFAULT 'root',
  ssh_password TEXT NOT NULL DEFAULT '',
  prefer_port INTEGER NOT NULL DEFAULT 18080,
  proxy_port INTEGER NOT NULL DEFAULT 0,
  proxy_user TEXT NOT NULL DEFAULT 'sendsmtp',
  proxy_password TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  fail_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  sent_count INTEGER NOT NULL DEFAULT 0,
  UNIQUE(host)
)`,
	} {
		_, _ = s.db.Exec(q) // ignore "duplicate column" / already exists
	}
	return s.ensureEmailCounts()
}

// RebuildEmailCounts recomputes pending/sending/sent/failed from emails (one GROUP BY).
func (s *Store) RebuildEmailCounts() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM email_counts`); err != nil {
		return err
	}
	for _, st := range []string{"pending", "sending", "sent", "failed"} {
		if _, err := tx.Exec(`INSERT INTO email_counts (status, n) VALUES (?, 0)`, st); err != nil {
			return err
		}
	}
	rows, err := tx.Query(`SELECT status, COUNT(*) FROM emails GROUP BY status`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO email_counts (status, n) VALUES (?, ?)
			ON CONFLICT(status) DO UPDATE SET n=excluded.n`, st, n); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ensureEmailCounts() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM email_counts`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return s.RebuildEmailCounts()
	}
	return nil
}

func (s *Store) adjustEmailCount(status string, delta int64) error {
	if delta == 0 || status == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE email_counts SET n = MAX(0, n + ?) WHERE status=?`, delta, status)
	return err
}

func adjustEmailCountTx(tx *sql.Tx, status string, delta int64) error {
	if delta == 0 || status == "" {
		return nil
	}
	_, err := tx.Exec(`UPDATE email_counts SET n = MAX(0, n + ?) WHERE status=?`, delta, status)
	return err
}

func moveEmailCountTx(tx *sql.Tx, from, to string) error {
	if err := adjustEmailCountTx(tx, from, -1); err != nil {
		return err
	}
	return adjustEmailCountTx(tx, to, 1)
}

func (s *Store) ReopenOrphans() (int64, error) {
	res, err := s.db.Exec(`UPDATE emails SET status='pending', updated_at=datetime('now') WHERE status='sending'`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_ = s.adjustEmailCount("sending", -n)
		_ = s.adjustEmailCount("pending", n)
	}
	return n, nil
}

func (s *Store) UpsertSMTPs(list []SMTP) (inserted, updated int, err error) {
	return s.UpsertSMTPsProgress(list, nil)
}

func (s *Store) UpsertSMTPsProgress(list []SMTP, onProgress func(done, total int)) (inserted, updated int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	total := len(list)
	for i, a := range list {
		var id int64
		err := tx.QueryRow(`SELECT id FROM smtps WHERE host=? AND user=?`, a.Host, a.User).Scan(&id)
		if err == sql.ErrNoRows {
			_, err = tx.Exec(`INSERT INTO smtps (domain, account_label, host, port, encryption, from_addr, user, password, status)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active')`,
				a.Domain, a.AccountLabel, a.Host, a.Port, a.Encryption, a.FromAddr, a.User, a.Password)
			if err != nil {
				return inserted, updated, err
			}
			inserted++
		} else if err != nil {
			return inserted, updated, err
		} else {
			_, err = tx.Exec(`UPDATE smtps SET domain=?, account_label=?, port=?, encryption=?, from_addr=?, password=?, status='active', fail_count=0, last_error='' WHERE id=?`,
				a.Domain, a.AccountLabel, a.Port, a.Encryption, a.FromAddr, a.Password, id)
			if err != nil {
				return inserted, updated, err
			}
			updated++
		}
		if onProgress != nil && (i%10 == 0 || i+1 == total) {
			onProgress(i+1, total)
		}
	}
	if err := tx.Commit(); err != nil {
		return inserted, updated, err
	}
	return inserted, updated, nil
}

func (s *Store) ListSMTPs() ([]SMTP, error) {
	rows, err := s.db.Query(`SELECT id, domain, account_label, host, port, encryption, from_addr, user, password, status, fail_count, last_error, sent_count,
		inbox_score, inbox_rate, inbox_label, spam_test_id, spam_checked_at, spam_summary FROM smtps ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SMTP
	for rows.Next() {
		var a SMTP
		if err := scanSMTP(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetSMTP(id int64) (SMTP, error) {
	var a SMTP
	row := s.db.QueryRow(`SELECT id, domain, account_label, host, port, encryption, from_addr, user, password, status, fail_count, last_error, sent_count,
		inbox_score, inbox_rate, inbox_label, spam_test_id, spam_checked_at, spam_summary FROM smtps WHERE id=?`, id)
	err := scanSMTP(row, &a)
	return a, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSMTP(r rowScanner, a *SMTP) error {
	return r.Scan(
		&a.ID, &a.Domain, &a.AccountLabel, &a.Host, &a.Port, &a.Encryption, &a.FromAddr, &a.User, &a.Password,
		&a.Status, &a.FailCount, &a.LastError, &a.SentCount,
		&a.InboxScore, &a.InboxRate, &a.InboxLabel, &a.SpamTestID, &a.SpamCheckedAt, &a.SpamSummary,
	)
}

func (s *Store) UpdateSMTPSpamResult(id int64, score, rate float64, label, testID, summary string) error {
	_, err := s.db.Exec(`UPDATE smtps SET inbox_score=?, inbox_rate=?, inbox_label=?, spam_test_id=?, spam_summary=?, spam_checked_at=datetime('now') WHERE id=?`,
		score, rate, label, testID, summary, id)
	return err
}

func (s *Store) SetSMTPStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE smtps SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) RecordSMTPSuccess(id int64) error {
	_, err := s.db.Exec(`UPDATE smtps SET fail_count=0, last_error='', sent_count=sent_count+1 WHERE id=?`, id)
	return err
}

func (s *Store) RecordSMTPFailure(id int64, errMsg string, disableAfter int) (disabled bool, err error) {
	_, err = s.db.Exec(`UPDATE smtps SET fail_count=fail_count+1, last_error=? WHERE id=?`, errMsg, id)
	if err != nil {
		return false, err
	}
	var failCount int
	if err = s.db.QueryRow(`SELECT fail_count FROM smtps WHERE id=?`, id).Scan(&failCount); err != nil {
		return false, err
	}
	if disableAfter > 0 && failCount >= disableAfter {
		if err = s.SetSMTPStatus(id, "disabled"); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *Store) PickActiveSMTP(offset int) (SMTP, error) {
	var a SMTP
	err := scanSMTP(s.db.QueryRow(`SELECT id, domain, account_label, host, port, encryption, from_addr, user, password, status, fail_count, last_error, sent_count,
		inbox_score, inbox_rate, inbox_label, spam_test_id, spam_checked_at, spam_summary
		FROM smtps WHERE status='active' ORDER BY fail_count ASC, sent_count ASC, id ASC LIMIT 1 OFFSET ?`, offset), &a)
	if err == sql.ErrNoRows {
		err = scanSMTP(s.db.QueryRow(`SELECT id, domain, account_label, host, port, encryption, from_addr, user, password, status, fail_count, last_error, sent_count,
			inbox_score, inbox_rate, inbox_label, spam_test_id, spam_checked_at, spam_summary
			FROM smtps WHERE status='active' ORDER BY fail_count ASC, sent_count ASC, id ASC LIMIT 1`), &a)
	}
	return a, err
}

func (s *Store) CountActiveSMTPs() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM smtps WHERE status='active'`).Scan(&n)
	return n, err
}

func (s *Store) ImportEmails(addresses []string) (inserted, skipped int, err error) {
	return s.ImportEmailsProgress(addresses, nil)
}

func (s *Store) ImportEmailsProgress(addresses []string, onProgress func(done, total int)) (inserted, skipped int, err error) {
	const chunk = 5000
	total := len(addresses)
	for start := 0; start < total; start += chunk {
		end := start + chunk
		if end > total {
			end = total
		}
		tx, err := s.db.Begin()
		if err != nil {
			return inserted, skipped, err
		}
		stmt, err := tx.Prepare(`INSERT OR IGNORE INTO emails (address, status) VALUES (?, 'pending')`)
		if err != nil {
			_ = tx.Rollback()
			return inserted, skipped, err
		}
		chunkIns := 0
		for i := start; i < end; i++ {
			addr := emailvalid.Normalize(addresses[i])
			if addr == "" || !strings.Contains(addr, "@") {
				skipped++
			} else {
				res, err := stmt.Exec(addr)
				if err != nil {
					stmt.Close()
					_ = tx.Rollback()
					return inserted, skipped, err
				}
				n, _ := res.RowsAffected()
				if n > 0 {
					inserted++
					chunkIns++
				} else {
					skipped++
				}
			}
			if onProgress != nil && (i%200 == 0 || i+1 == total) {
				onProgress(i+1, total)
			}
		}
		stmt.Close()
		if chunkIns > 0 {
			if err := adjustEmailCountTx(tx, "pending", int64(chunkIns)); err != nil {
				_ = tx.Rollback()
				return inserted, skipped, err
			}
		}
		if err := tx.Commit(); err != nil {
			return inserted, skipped, err
		}
	}
	return inserted, skipped, nil
}

// statusRank prefers sent over in-flight over pending over failed when merging duplicates.
func statusRank(status string) int {
	switch status {
	case "sent":
		return 4
	case "sending":
		return 3
	case "pending":
		return 2
	case "failed":
		return 1
	default:
		return 0
	}
}

// DedupeNormalizedEmails merges rows that normalize to the same address (casing/quotes),
// keeps the best status, and rewrites the keeper to the canonical form.
// Prevents double-sends when the same mailbox was stored under slightly different strings.
func (s *Store) DedupeNormalizedEmails() (removed int64, err error) {
	rows, err := s.db.Query(`SELECT id, address, status FROM emails ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type row struct {
		id      int64
		address string
		status  string
	}
	byNorm := map[string]row{}
	var deleteIDs []int64

	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.address, &r.status); err != nil {
			return 0, err
		}
		norm := emailvalid.Normalize(r.address)
		if norm == "" {
			deleteIDs = append(deleteIDs, r.id)
			continue
		}
		prev, ok := byNorm[norm]
		if !ok {
			byNorm[norm] = r
			continue
		}
		// Keep the better status; on tie keep older (lower) id.
		keep, drop := prev, r
		if statusRank(r.status) > statusRank(prev.status) {
			keep, drop = r, prev
		}
		deleteIDs = append(deleteIDs, drop.id)
		byNorm[norm] = keep
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(deleteIDs) == 0 {
		// Still rewrite non-canonical addresses.
		needRewrite := false
		for norm, r := range byNorm {
			if r.address != norm {
				needRewrite = true
				break
			}
		}
		if !needRewrite {
			return 0, nil
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, id := range deleteIDs {
		if _, err := tx.Exec(`DELETE FROM emails WHERE id=?`, id); err != nil {
			return 0, err
		}
		removed++
	}
	for norm, r := range byNorm {
		if r.address == norm {
			continue
		}
		if _, err := tx.Exec(`UPDATE OR IGNORE emails SET address=?, updated_at=datetime('now') WHERE id=?`, norm, r.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	_ = s.RebuildEmailCounts()
	return removed, nil
}

func (s *Store) ReplaceSubjects(items []string) error {
	return s.replaceList("subjects", "text", items)
}

func (s *Store) ReplaceLinks(items []string) error {
	return s.replaceList("links", "url", items)
}

func (s *Store) replaceList(table, col string, items []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
		return err
	}
	stmt, err := tx.Prepare(fmt.Sprintf(`INSERT OR IGNORE INTO %s (%s) VALUES (?)`, table, col))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if _, err := stmt.Exec(it); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListSubjects() ([]string, error) {
	return s.listCol("subjects", "text")
}

func (s *Store) ListLinks() ([]string, error) {
	return s.listCol("links", "url")
}

func (s *Store) listCol(table, col string) ([]string, error) {
	rows, err := s.db.Query(fmt.Sprintf(`SELECT %s FROM %s ORDER BY id`, col, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) RandomSubject() (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT text FROM subjects ORDER BY RANDOM() LIMIT 1`).Scan(&v)
	return v, err
}

func (s *Store) RandomLink() (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT url FROM links ORDER BY RANDOM() LIMIT 1`).Scan(&v)
	return v, err
}

func (s *Store) ClaimPendingEmail() (Email, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Email{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var e Email
	err = tx.QueryRow(`SELECT id, address, status, attempts, smtp_id, error, subject, link, updated_at FROM emails WHERE status='pending' ORDER BY id LIMIT 1`).
		Scan(&e.ID, &e.Address, &e.Status, &e.Attempts, &e.SMTPID, &e.Error, &e.Subject, &e.Link, &e.UpdatedAt)
	if err != nil {
		return Email{}, err
	}
	res, err := tx.Exec(`UPDATE emails SET status='sending', attempts=attempts+1, updated_at=datetime('now') WHERE id=? AND status='pending'`, e.ID)
	if err != nil {
		return Email{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Email{}, sql.ErrNoRows
	}
	if err := moveEmailCountTx(tx, "pending", "sending"); err != nil {
		return Email{}, err
	}
	e.Status = "sending"
	e.Attempts++
	e.UpdatedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	if err := tx.Commit(); err != nil {
		return Email{}, err
	}
	return e, nil
}

func (s *Store) MarkEmailSent(id, smtpID int64, subject, link string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE emails SET status='sent', smtp_id=?, error='', subject=?, link=?, updated_at=datetime('now') WHERE id=? AND status='sending'`,
		smtpID, subject, link, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if err := moveEmailCountTx(tx, "sending", "sent"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MarkEmailFailed(id int64, errMsg string, retry bool) error {
	status := "failed"
	if retry {
		status = "pending"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE emails SET status=?, error=?, updated_at=datetime('now') WHERE id=? AND status='sending'`, status, errMsg, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if err := moveEmailCountTx(tx, "sending", status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReleaseToPending puts a claimed email back without treating it as a hard failure.
func (s *Store) ReleaseToPending(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE emails SET status='pending', attempts=CASE WHEN attempts>0 THEN attempts-1 ELSE 0 END, updated_at=datetime('now') WHERE id=? AND status='sending'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if err := moveEmailCountTx(tx, "sending", "pending"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ResetFailed() (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE emails SET status='pending', error='', updated_at=datetime('now') WHERE status='failed'`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		if err := adjustEmailCountTx(tx, "failed", -n); err != nil {
			return 0, err
		}
		if err := adjustEmailCountTx(tx, "pending", n); err != nil {
			return 0, err
		}
	}
	return n, tx.Commit()
}

// DeleteAllEmails removes every recipient from the queue.
func (s *Store) DeleteAllEmails() (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM emails`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := tx.Exec(`UPDATE email_counts SET n=0`); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// ClearErrorLogs clears error text on emails and SMTPs (does not change queue status).
func (s *Store) ClearErrorLogs() (emails, smtps int64, err error) {
	res, err := s.db.Exec(`UPDATE emails SET error='' WHERE error!=''`)
	if err != nil {
		return 0, 0, err
	}
	emails, _ = res.RowsAffected()
	res, err = s.db.Exec(`UPDATE smtps SET last_error='', fail_count=0`)
	if err != nil {
		return emails, 0, err
	}
	smtps, _ = res.RowsAffected()
	return emails, smtps, nil
}

// ReenableAllSMTPs marks every SMTP active and clears fail counters.
func (s *Store) ReenableAllSMTPs() (int64, error) {
	res, err := s.db.Exec(`UPDATE smtps SET status='active', fail_count=0, last_error=''`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ReenableCooldownSMTPs reactivates disabled SMTPs that look like transient timeouts.
func (s *Store) ReenableCooldownSMTPs() (int64, error) {
	res, err := s.db.Exec(`UPDATE smtps SET status='active', fail_count=0
		WHERE status='disabled' AND (
			last_error LIKE '%timeout%' OR
			last_error LIKE '%i/o timeout%' OR
			last_error LIKE '%connection refused%' OR
			last_error LIKE '%connection reset%' OR
			last_error LIKE '%EOF%' OR
			last_error LIKE '%421%' OR
			last_error LIKE '%450%' OR
			last_error LIKE '%451%'
		)`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type SMTPStat struct {
	ID         int64   `json:"id"`
	Host       string  `json:"host"`
	Status     string  `json:"status"`
	SentCount  int64   `json:"sent_count"`
	FailCount  int     `json:"fail_count"`
	LastError  string  `json:"last_error"`
	InboxLabel string  `json:"inbox_label"`
	InboxScore float64 `json:"inbox_score"`
	InboxRate  float64 `json:"inbox_rate"`
}

func (s *Store) SMTPStats() ([]SMTPStat, error) {
	rows, err := s.db.Query(`SELECT id, host, status, sent_count, fail_count, last_error, inbox_label, inbox_score, inbox_rate FROM smtps ORDER BY sent_count DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SMTPStat
	for rows.Next() {
		var a SMTPStat
		if err := rows.Scan(&a.ID, &a.Host, &a.Status, &a.SentCount, &a.FailCount, &a.LastError, &a.InboxLabel, &a.InboxScore, &a.InboxRate); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListEmails(status string, limit, offset int) ([]Email, error) {
	page, err := s.ListEmailsPage(status, "", limit, offset)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Store) ListEmailsPage(status, query string, limit, offset int) (EmailPage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	where := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if status != "" && status != "all" {
		where = append(where, "status=?")
		args = append(args, status)
	}
	q := strings.TrimSpace(strings.ToLower(query))
	if q != "" {
		// Addresses are stored normalized (lowercase); avoid lower() so the planner stays simple.
		where = append(where, "address LIKE ?")
		args = append(args, "%"+q+"%")
	}
	clause := "1=1"
	if len(where) > 0 {
		clause = strings.Join(where, " AND ")
	}

	var total int64
	if q == "" {
		// O(1) from email_counts when not searching.
		st, err := s.emailStatusTotals()
		if err != nil {
			return EmailPage{}, err
		}
		switch status {
		case "", "all":
			total = st.Pending + st.Sending + st.Sent + st.Failed
		case "pending":
			total = st.Pending
		case "sending":
			total = st.Sending
		case "sent":
			total = st.Sent
		case "failed":
			total = st.Failed
		default:
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE `+clause, args...).Scan(&total); err != nil {
				return EmailPage{}, err
			}
		}
	} else if err := s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE `+clause, args...).Scan(&total); err != nil {
		return EmailPage{}, err
	}

	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.Query(
		`SELECT id, address, status, attempts, smtp_id, error, subject, link, updated_at FROM emails WHERE `+clause+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return EmailPage{}, err
	}
	defer rows.Close()

	out := make([]Email, 0, limit)
	for rows.Next() {
		var e Email
		if err := rows.Scan(&e.ID, &e.Address, &e.Status, &e.Attempts, &e.SMTPID, &e.Error, &e.Subject, &e.Link, &e.UpdatedAt); err != nil {
			return EmailPage{}, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return EmailPage{}, err
	}
	return EmailPage{Items: out, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Store) emailStatusTotals() (StatusCounts, error) {
	var c StatusCounts
	rows, err := s.db.Query(`SELECT status, n FROM email_counts`)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			return c, err
		}
		switch st {
		case "pending":
			c.Pending = n
		case "sending":
			c.Sending = n
		case "sent":
			c.Sent = n
		case "failed":
			c.Failed = n
		}
	}
	return c, rows.Err()
}

func (s *Store) GetStatus() (StatusCounts, error) {
	c, err := s.emailStatusTotals()
	if err != nil {
		return c, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM smtps WHERE status='active'`).Scan(&c.SMTPsActive); err != nil {
		return c, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM smtps WHERE status='disabled'`).Scan(&c.SMTPsDisabled); err != nil {
		return c, err
	}
	return c, nil
}

// QueueIdle is a cheap check used by the campaign loop (index seek, not COUNT).
func (s *Store) QueueIdle() (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM emails WHERE status IN ('pending','sending') LIMIT 1`).Scan(&one)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

func (s *Store) GetCampaign() (Campaign, error) {
	var c Campaign
	err := s.db.QueryRow(`SELECT id, state, html, from_name, started_at, updated_at FROM campaigns WHERE id=1`).
		Scan(&c.ID, &c.State, &c.HTML, &c.FromName, &c.StartedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) SetCampaignState(state string) error {
	_, err := s.db.Exec(`UPDATE campaigns SET state=?, updated_at=datetime('now') WHERE id=1`, state)
	return err
}

func (s *Store) SetCampaignHTML(html, fromName string) error {
	_, err := s.db.Exec(`UPDATE campaigns SET html=?, from_name=?, updated_at=datetime('now') WHERE id=1`, html, fromName)
	return err
}

func (s *Store) StartCampaign() error {
	_, err := s.db.Exec(`UPDATE campaigns SET state='running', started_at=datetime('now'), updated_at=datetime('now') WHERE id=1`)
	return err
}

func (s *Store) GetHTML() (string, error) {
	var html string
	err := s.db.QueryRow(`SELECT html FROM campaigns WHERE id=1`).Scan(&html)
	return html, err
}

func (s *Store) TopErrors(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}
	s.topErrMu.Lock()
	if time.Since(s.topErrAt) < 15*time.Second && s.topErrCache != nil {
		out := append([]string(nil), s.topErrCache...)
		s.topErrMu.Unlock()
		return out, nil
	}
	s.topErrMu.Unlock()

	rows, err := s.db.Query(`SELECT error, COUNT(*) AS n FROM emails WHERE status='failed' AND error!='' GROUP BY error ORDER BY n DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var msg string
		var n int
		if err := rows.Scan(&msg, &n); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%d× %s", n, msg))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.topErrMu.Lock()
	s.topErrCache = append([]string(nil), out...)
	s.topErrAt = time.Now()
	s.topErrMu.Unlock()
	return out, nil
}
