package email

import (
	"context"
	"log"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// LogMailer is the default Mailer adapter for dev/local/CI. It renders the
// template and logs a non-PII summary (recipient count, template, locale,
// subject) instead of delivering, so `docker compose` and tests need no SMTP
// server or credentials. It never returns an error for a valid template.
type LogMailer struct{}

// NewLogMailer constructs the logging Mailer.
func NewLogMailer() *LogMailer { return &LogMailer{} }

var _ ports.Mailer = (*LogMailer)(nil)

// Send renders the message and logs a summary. To avoid leaking PII, it logs the
// recipient count (not addresses) and the subject only — never the body.
func (m *LogMailer) Send(ctx context.Context, msg domain.EmailMessage) error {
	locale := resolveLocale(msg.Locale)
	r, err := render(msg.Template, locale, msg.Data)
	if err != nil {
		return err
	}
	log.Printf("email(log): template=%s locale=%s recipients=%d subject=%q",
		msg.Template, locale, len(msg.To), r.Subject)
	return nil
}
