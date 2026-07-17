package parser

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const smtpBlockHeader = "--- SMTP config (goscan) ---"

// SMTPAccount is a parsed goscan SMTP block.
type SMTPAccount struct {
	Domain       string
	AccountLabel string
	Host         string
	Port         int
	Encryption   string // tls | starttls | none | ssl
	From         string
	User         string
	Password     string
}

// ParseGoscanSMTPs parses one or more goscan SMTP blocks from text.
func ParseGoscanSMTPs(raw string) ([]SMTPAccount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var accounts []SMTPAccount
	var current map[string]string
	flush := func() error {
		if current == nil {
			return nil
		}
		acc, err := mapToAccount(current)
		if err != nil {
			return err
		}
		accounts = append(accounts, acc)
		current = nil
		return nil
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "---") && strings.Contains(strings.ToLower(line), "smtp") {
			if err := flush(); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			current = map[string]string{}
			continue
		}
		if current == nil {
			// Allow first block without explicit header if key:value present
			if strings.Contains(line, ":") {
				current = map[string]string{}
			} else {
				continue
			}
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		current[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(val)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func mapToAccount(m map[string]string) (SMTPAccount, error) {
	acc := SMTPAccount{
		Domain:       m["domain"],
		AccountLabel: m["account_label"],
		Host:         m["host"],
		Encryption:   strings.ToLower(m["encryption"]),
		From:         m["from"],
		User:         m["user"],
		Password:     m["password"],
	}
	if acc.Host == "" {
		return acc, fmt.Errorf("smtp missing host")
	}
	if acc.User == "" {
		return acc, fmt.Errorf("smtp %s missing user", acc.Host)
	}
	portStr := m["port"]
	if portStr == "" {
		acc.Port = 587
	} else {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 {
			return acc, fmt.Errorf("smtp %s invalid port %q", acc.Host, portStr)
		}
		acc.Port = p
	}
	if acc.Encryption == "" {
		acc.Encryption = "tls"
	}
	switch acc.Encryption {
	case "tls", "ssl", "starttls", "none", "plain":
	default:
		return acc, fmt.Errorf("smtp %s unknown encryption %q", acc.Host, acc.Encryption)
	}
	if acc.From == "" {
		acc.From = acc.User
	}
	_ = smtpBlockHeader // referenced for clarity / docs
	return acc, nil
}

// ParseEmailPasswordLines parses lines like:
//
//	atendimento@creluz.com.br;@Creluz2026
//
// user/from = email, password = everything after the first ';'.
// Skips blanks, # comments, and goscan-looking lines.
func ParseEmailPasswordLines(raw string) []SMTPAccount {
	var out []SMTPAccount
	for _, line := range ParseLines(raw) {
		if looksLikeGoscanLine(line) {
			continue
		}
		email, pass, ok := splitEmailPassword(line)
		if !ok {
			continue
		}
		dom := ""
		if at := strings.LastIndex(email, "@"); at > 0 {
			dom = strings.ToLower(email[at+1:])
		}
		out = append(out, SMTPAccount{
			Domain:       dom,
			AccountLabel: "auto",
			From:         email,
			User:         email,
			Password:     pass,
			Port:         587,
			Encryption:   "starttls",
		})
	}
	return out
}

func splitEmailPassword(line string) (email, password string, ok bool) {
	email, password, cut := strings.Cut(line, ";")
	if !cut {
		return "", "", false
	}
	email = strings.TrimSpace(strings.ToLower(email))
	password = strings.TrimSpace(password) // keep leading @
	if password == "" || !strings.Contains(email, "@") {
		return "", "", false
	}
	at := strings.LastIndex(email, "@")
	if at < 1 || at == len(email)-1 {
		return "", "", false
	}
	if strings.Contains(email, " ") {
		return "", "", false
	}
	return email, password, true
}

func looksLikeGoscanLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.HasPrefix(line, "---") {
		return true
	}
	// key: value without being email;pass
	if strings.Contains(line, ":") && !strings.Contains(line, ";") {
		key, _, ok := strings.Cut(lower, ":")
		if !ok {
			return false
		}
		key = strings.TrimSpace(key)
		switch key {
		case "domain", "account_label", "host", "port", "encryption", "from", "user", "password":
			return true
		}
	}
	return false
}

// LooksLikeGoscan reports whether raw is primarily goscan block format.
func LooksLikeGoscan(raw string) bool {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "smtp config") || strings.Contains(lower, "--- smtp") {
		return true
	}
	return strings.Contains(lower, "host:") && (strings.Contains(lower, "user:") || strings.Contains(lower, "password:"))
}

// ParseLines returns non-empty trimmed lines, skipping # comments.
func ParseLines(raw string) []string {
	return ParseLinesReader(strings.NewReader(raw))
}

// ParseLinesReader streams lines from r (suitable for large files).
func ParseLinesReader(r io.Reader) []string {
	var out []string
	br := bufio.NewReaderSize(r, 256*1024)
	for {
		line, err := br.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
	return out
}
