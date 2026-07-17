package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/wiz/sendsmtp/internal/store"
)

type Message struct {
	FromName string
	From     string
	To       string
	Subject  string
	HTML     string
}

func BuildMIME(msg Message) []byte {
	fromHeader := msg.From
	if msg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", sanitizeHeader(msg.FromName), msg.From)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	b.WriteString(fmt.Sprintf("To: %s\r\n", msg.To))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", sanitizeHeader(msg.Subject)))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.HTML)
	return []byte(b.String())
}

func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// Send delivers a message using the given SMTP account.
func Send(acc store.SMTP, msg Message, dialTimeout, sendTimeout time.Duration) error {
	addr := fmt.Sprintf("%s:%d", acc.Host, acc.Port)
	raw := BuildMIME(msg)

	switch strings.ToLower(acc.Encryption) {
	case "ssl", "tls":
		// Implicit TLS (typically 465). Also used when user labels STARTTLS port as "tls".
		if acc.Port == 587 {
			return sendSTARTTLS(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout)
		}
		return sendImplicitTLS(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout)
	case "starttls":
		return sendSTARTTLS(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout)
	default:
		return sendPlain(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout)
	}
}

func sendImplicitTLS(acc store.SMTP, addr, from, to string, raw []byte, dialTimeout, sendTimeout time.Duration) error {
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: acc.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(sendTimeout))
	client, err := smtp.NewClient(conn, acc.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	return smtpSend(client, acc, from, to, raw)
}

func sendSTARTTLS(acc store.SMTP, addr, from, to string, raw []byte, dialTimeout, sendTimeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(sendTimeout))
	client, err := smtp.NewClient(conn, acc.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: acc.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	return smtpSend(client, acc, from, to, raw)
}

func sendPlain(acc store.SMTP, addr, from, to string, raw []byte, dialTimeout, sendTimeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(sendTimeout))
	client, err := smtp.NewClient(conn, acc.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	return smtpSend(client, acc, from, to, raw)
}

func smtpSend(client *smtp.Client, acc store.SMTP, from, to string, raw []byte) error {
	if ok, _ := client.Extension("AUTH"); ok && acc.User != "" {
		auth := smtp.PlainAuth("", acc.User, acc.Password, acc.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	return client.Quit()
}
