package ports

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
)

type ConversationRepo interface {
	Create(ctx context.Context, channel string) (*domain.Conversation, error)
	GetByID(ctx context.Context, id string) (*domain.Conversation, error)
	UpdateState(ctx context.Context, id string, state domain.ConversationState) error
	UpdateExtracted(ctx context.Context, id string, extracted map[string]any) error
}

type MessageRepo interface {
	Append(ctx context.Context, msg *domain.Message) error
	ListByConversation(ctx context.Context, conversationID string) ([]*domain.Message, error)
	// SeenProviderMsg reports whether an inbound message with this provider id has
	// already been recorded. It backs idempotency: a redelivered inbound message
	// is processed at most once. Implementations rely on the partial-unique index
	// messages_provider_msg_uq (migration 018).
	SeenProviderMsg(ctx context.Context, providerMsgID string) (bool, error)
	// ClaimProviderMsg atomically records an inbound provider message id for a
	// conversation and reports whether this caller won the claim (true = first
	// time, proceed; false = a concurrent/earlier turn already claimed it, skip).
	// It is implemented as an insert that defers to the partial-unique index, so
	// the check-and-mark is a single atomic operation safe under at-least-once
	// delivery. providerMsgID must be non-empty.
	ClaimProviderMsg(ctx context.Context, conversationID, providerMsgID, content string) (claimed bool, err error)
}

type LeadRepo interface {
	Create(ctx context.Context, lead *domain.Lead) error
	GetByConversationID(ctx context.Context, convID string) (*domain.Lead, error)
	UpdateCompanyContact(ctx context.Context, leadID, companyID, contactID string) error
	List(ctx context.Context) ([]*domain.LeadSummary, error)
	GetByID(ctx context.Context, id string) (*domain.LeadDetail, error)
}

type CompanyRepo interface {
	GetByCUI(ctx context.Context, cui string) (*domain.Company, error)
	Upsert(ctx context.Context, company *domain.Company) error
}

type ContactRepo interface {
	Create(ctx context.Context, contact *domain.Contact) error
	Update(ctx context.Context, contact *domain.Contact) error
}

type SourcingRepo interface {
	Create(ctx context.Context, req *domain.SourcingRequest) error
	UpdateByLeadID(ctx context.Context, req *domain.SourcingRequest) error
}
