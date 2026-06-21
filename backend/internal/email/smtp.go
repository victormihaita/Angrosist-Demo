package email

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net/smtp"
	"strings"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// SMTPConfig is the SMTP adapter configuration. Every field comes from env via
// the composition root (internal/app) — nothing is hardcoded. Any EU
// transactional provider that speaks SMTP (Brevo, Mailgun, SendGrid, ...) works
// through these settings; no vendor SDK is needed.
type SMTPConfig struct {
	Host     string // SMTP_HOST, e.g. smtp-relay.brevo.com
	Port     string // SMTP_PORT, e.g. 587
	User     string // SMTP_USER (provider login)
	Password string // SMTP_PASSWORD (provider key) — from Secret Manager in prod
	From     string // MAIL_FROM, the envelope/header From address
}

// SMTPMailer is the production-capable Mailer adapter. It renders the template
// and delivers a multipart-free HTML+text message over SMTP with STARTTLS. It
// satisfies ports.Mailer.
type SMTPMailer struct {
	cfg  SMTPConfig
	send func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewSMTPMailer constructs the SMTP Mailer. It returns an error if required
// configuration (host, port, from) is missing, so the composition root fails
// fast rather than silently dropping mail.
func NewSMTPMailer(cfg SMTPConfig) (*SMTPMailer, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, fmt.Errorf("smtp mailer: SMTP_HOST is required")
	}
	if strings.TrimSpace(cfg.Port) == "" {
		return nil, fmt.Errorf("smtp mailer: SMTP_PORT is required")
	}
	if strings.TrimSpace(cfg.From) == "" {
		return nil, fmt.Errorf("smtp mailer: MAIL_FROM is required")
	}
	return &SMTPMailer{cfg: cfg, send: sendMailStartTLS}, nil
}

var _ ports.Mailer = (*SMTPMailer)(nil)

// Send renders the message and delivers it over SMTP. A nil-recipient message is
// skipped (no error). Errors are wrapped with context but carry no PII.
func (m *SMTPMailer) Send(ctx context.Context, msg domain.EmailMessage) error {
	if len(msg.To) == 0 {
		return nil
	}
	locale := resolveLocale(msg.Locale)
	r, err := render(msg.Template, locale, msg.Data)
	if err != nil {
		return err
	}

	raw := buildMIME(m.cfg.From, msg.To, msg.ReplyTo, r)
	addr := m.cfg.Host + ":" + m.cfg.Port
	auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)

	if err := m.send(addr, auth, m.cfg.From, msg.To, raw); err != nil {
		return fmt.Errorf("smtp send template=%s recipients=%d: %w", msg.Template, len(msg.To), err)
	}
	log.Printf("email(smtp): sent template=%s locale=%s recipients=%d", msg.Template, locale, len(msg.To))
	return nil
}

// buildMIME assembles a minimal HTML email (with a plain-text alternative) as
// raw RFC 5322 bytes. CRLF line endings are required by SMTP.
func buildMIME(from string, to []string, replyTo string, r rendered) []byte {
	boundary := "euro-intermed-boundary"
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if strings.TrimSpace(replyTo) != "" {
		b.WriteString("Reply-To: " + replyTo + "\r\n")
	}
	b.WriteString("Subject: " + encodeHeader(r.Subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	b.WriteString("\r\n")

	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(r.Text + "\r\n")

	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(r.HTML + "\r\n")

	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

// encodeHeader RFC 2047-encodes a subject when it contains non-ASCII, so
// diacritics in RO subjects survive transport.
func encodeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}

// sendMailStartTLS dials the SMTP server, upgrades to TLS via STARTTLS,
// authenticates, and sends. It is the production wiring for SMTPMailer.send;
// tests substitute a fake to avoid network I/O.
func sendMailStartTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}

	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("auth: %w", err)
			}
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close body: %w", err)
	}
	return c.Quit()
}
