package ports

import (
	"context"

	"github.com/angrosist/demo/pkg/domain"
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
}

type LeadRepo interface {
	Create(ctx context.Context, lead *domain.Lead) error
	List(ctx context.Context) ([]*domain.LeadSummary, error)
	GetByID(ctx context.Context, id string) (*domain.LeadDetail, error)
}

type CompanyRepo interface {
	GetByCUI(ctx context.Context, cui string) (*domain.Company, error)
	Upsert(ctx context.Context, company *domain.Company) error
}

type ContactRepo interface {
	Create(ctx context.Context, contact *domain.Contact) error
}

type SourcingRepo interface {
	Create(ctx context.Context, req *domain.SourcingRequest) error
}
