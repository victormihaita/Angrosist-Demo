package ports

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
)

// CompanyDataProvider verifies a Romanian company by CUI via an external service.
type CompanyDataProvider interface {
	Verify(ctx context.Context, cui string) (*domain.Company, error)
}

// AgentRunner executes one conversational turn and returns the assistant's reply.
type AgentRunner interface {
	RunTurn(ctx context.Context, conversationID string, userMessage string) (reply string, err error)
}

// Mailer is the single seam to transactional email. It renders the named
// template in the requested locale and delivers it through whatever provider the
// wired adapter uses (log adapter for dev/tests, SMTP for prod). The inner
// layers depend only on this interface, so swapping providers — or adding an
// API-based adapter later — is a wiring change, never a domain change.
//
// Send is best-effort from the caller's perspective on the agent hot path: a
// delivery failure is logged and must not fail the conversational turn. The
// adapter still returns the error so callers that care (or future retry logic)
// can observe it.
type Mailer interface {
	Send(ctx context.Context, msg domain.EmailMessage) error
}
