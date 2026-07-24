package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Account is the SMTP credentials needed to deliver a message.
type Account struct {
	Host       string
	Port       int
	Encryption string
	User       string
	Password   string
	FromAddr   string
}

type Message struct {
	FromName string
	From     string
	To       string
	Subject  string
	HTML     string
}

// DialFunc dials a network address (optionally through a proxy).
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

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

// Send delivers a message using the given SMTP account (direct dial).
func Send(acc Account, msg Message, dialTimeout, sendTimeout time.Duration) error {
	return SendDial(acc, msg, dialTimeout, sendTimeout, nil)
}

// SendDial is like Send but uses dial when non-nil.
func SendDial(acc Account, msg Message, dialTimeout, sendTimeout time.Duration, dial DialFunc) error {
	addr := fmt.Sprintf("%s:%d", acc.Host, acc.Port)
	raw := BuildMIME(msg)
	if dial == nil {
		dial = directDial(dialTimeout)
	}

	switch strings.ToLower(acc.Encryption) {
	case "ssl", "tls":
		if acc.Port == 587 {
			return sendSTARTTLS(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout, dial)
		}
		return sendImplicitTLS(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout, dial)
	case "starttls":
		return sendSTARTTLS(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout, dial)
	default:
		return sendPlain(acc, addr, msg.From, msg.To, raw, dialTimeout, sendTimeout, dial)
	}
}

func directDial(dialTimeout time.Duration) DialFunc {
	d := &net.Dialer{Timeout: dialTimeout}
	return d.DialContext
}

func dialTCP(dial DialFunc, addr string, dialTimeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	return dial(ctx, "tcp", addr)
}

func sendImplicitTLS(acc Account, addr, from, to string, raw []byte, dialTimeout, sendTimeout time.Duration, dial DialFunc) error {
	rawConn, err := dialTCP(dial, addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	conn := tls.Client(rawConn, &tls.Config{ServerName: acc.Host, MinVersion: tls.VersionTLS12})
	if err := conn.Handshake(); err != nil {
		_ = rawConn.Close()
		return fmt.Errorf("tls handshake: %w", err)
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

func sendSTARTTLS(acc Account, addr, from, to string, raw []byte, dialTimeout, sendTimeout time.Duration, dial DialFunc) error {
	conn, err := dialTCP(dial, addr, dialTimeout)
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

func sendPlain(acc Account, addr, from, to string, raw []byte, dialTimeout, sendTimeout time.Duration, dial DialFunc) error {
	conn, err := dialTCP(dial, addr, dialTimeout)
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

func smtpSend(client *smtp.Client, acc Account, from, to string, raw []byte) error {
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

// FromStore builds an Account from common SMTP row fields.
func FromStore(host string, port int, encryption, user, password, fromAddr string) Account {
	return Account{
		Host:       host,
		Port:       port,
		Encryption: encryption,
		User:       user,
		Password:   password,
		FromAddr:   fromAddr,
	}
}
