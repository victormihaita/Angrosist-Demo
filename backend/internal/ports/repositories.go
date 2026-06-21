package ports

import (
	"context"
	"errors"

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

	// ListPage returns one keyset page of LeadSummary rows matching the filter,
	// ordered created_at DESC, id DESC. It fetches filter.Limit+1 rows so the
	// caller can detect whether a further page exists.
	ListPage(ctx context.Context, filter domain.LeadFilter) ([]*domain.LeadSummary, error)
	// Handoffs returns one keyset page of leads needing a human, newest first,
	// each carrying a short snippet of the conversation's last message.
	Handoffs(ctx context.Context, filter domain.LeadFilter) ([]*domain.HandoffItem, error)
	// UpdateOffer applies a manual offer change (status/value/note) to the lead and
	// returns ErrNotFound when no such lead exists. Nil OfferUpdate fields are left
	// untouched. The status must already be validated against the lead_statuses
	// lookup by the caller.
	UpdateOffer(ctx context.Context, leadID string, upd domain.OfferUpdate) (*domain.LeadSummary, error)
	// Assign sets leads.assigned_to (nil unassigns) and returns ErrNotFound when no
	// such lead exists.
	Assign(ctx context.Context, leadID string, userID *string) (*domain.LeadSummary, error)
	// StatusExists reports whether the given code is a valid lead_statuses entry.
	StatusExists(ctx context.Context, status string) (bool, error)
	// KPIs computes the dashboard aggregate KPIs in a single pass.
	KPIs(ctx context.Context) (*domain.KPIs, error)
}

type CompanyRepo interface {
	GetByCUI(ctx context.Context, cui string) (*domain.Company, error)
	// GetByRegNo looks up a company by the canonical (country, reg_no) dedup key.
	GetByRegNo(ctx context.Context, country, regNo string) (*domain.Company, error)
	// Upsert dedups on (country, reg_no), persists the enriched columns, and —
	// when the company carries verification data — records a company_verifications
	// row for audit/cache.
	Upsert(ctx context.Context, company *domain.Company) error

	// ListPage returns one keyset page of directory companies matching the filter,
	// ordered created_at DESC, id DESC (filter.Limit+1 rows for has-next detection).
	ListPage(ctx context.Context, filter domain.CompanyFilter) ([]*domain.CompanySummary, error)
	// Detail returns the directory detail (roles, latest verification, financials)
	// for a company id, or ErrNotFound.
	Detail(ctx context.Context, id string) (*domain.CompanyDetail, error)
}

// ActivityLogRepo appends to the first-class audit log (migration 022). Audit
// rows are append-only and survive GDPR erasure (anonymized, never deleted).
type ActivityLogRepo interface {
	// Append writes one audit row. meta is serialized to JSONB; it must contain no
	// PII beyond ids.
	Append(ctx context.Context, entry domain.ActivityLog) error
}

// ErrNotFound is returned by repository reads/mutations when the addressed row
// does not exist. The dashboard handlers map it to a 404 envelope.
var ErrNotFound = errors.New("not found")

type ContactRepo interface {
	Create(ctx context.Context, contact *domain.Contact) error
	Update(ctx context.Context, contact *domain.Contact) error
}

type SourcingRepo interface {
	Create(ctx context.Context, req *domain.SourcingRequest) error
	UpdateByLeadID(ctx context.Context, req *domain.SourcingRequest) error
}

// UserRepo is the persistence port for dashboard operators (staff/admin). It is
// consumed by the auth/RBAC layer; the only production adapter lives in
// internal/persistence/postgres and a mock backs the unit tests. All lookups are
// parameterized; no SQL is constructed from caller input.
type UserRepo interface {
	// GetByID returns the user with the given id, or ErrUserNotFound if no such
	// row exists. The auth middleware uses it to resolve the JWT subject to the
	// live user on every request (so role changes take effect immediately).
	GetByID(ctx context.Context, id string) (*domain.User, error)
	// GetByEmail returns the user with the given email, or ErrUserNotFound if no
	// such row exists. The lookup is case-sensitive on the stored email.
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	// Create inserts a new user. The caller supplies Email, Name, Role and (for
	// password login) PasswordHash; the database assigns ID/CreatedAt/UpdatedAt,
	// which are written back onto the passed struct.
	Create(ctx context.Context, user *domain.User) error
	// List returns all users ordered by creation time (newest first). Intended for
	// the admin user-management screen; callers project to domain.PublicUser.
	List(ctx context.Context) ([]*domain.User, error)
	// UpsertByEmail inserts the user or, when one already exists with the same
	// email, updates its name, role and password hash. It backs idempotent admin
	// bootstrap and never duplicates rows on repeated startups.
	UpsertByEmail(ctx context.Context, user *domain.User) error
}

// ErrUserNotFound is returned by UserRepo.GetByEmail when no user matches. The
// auth layer maps it to a generic 401 so callers cannot distinguish "unknown
// email" from "wrong password".
var ErrUserNotFound = errors.New("user not found")
