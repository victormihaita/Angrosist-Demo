package ports

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
)

// CompanyVerifier verifies a Romanian company by CUI via an external service.
type CompanyVerifier interface {
	Verify(ctx context.Context, cui string) (*domain.Company, error)
}

// AgentRunner executes one conversational turn and returns the assistant's reply.
type AgentRunner interface {
	RunTurn(ctx context.Context, conversationID string, userMessage string) (reply string, err error)
}
