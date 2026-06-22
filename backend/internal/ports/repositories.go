package ports

import (
	"context"
	"errors"

	"github.com/angrosist/demo/internal/domain"
)

type ConversationRepo interface {
	Create(ctx context.Context, channel string) (*domain.Conversation, error)
	// CreateWith creates a conversation tagged with a vertical and intent (from the
	// verticals/intents lookups, migration 010), selecting the active flow. Empty
	// vertical/intent resolve to domain.DefaultVertical/DefaultIntent in the adapter
	// so the legacy Angrosist-buyer behavior is preserved.
	CreateWith(ctx context.Context, channel, vertical, intent string) (*domain.Conversation, error)
	GetByID(ctx context.Context, id string) (*domain.Conversation, error)
	UpdateState(ctx context.Context, id string, state domain.ConversationState) error
	UpdateExtracted(ctx context.Context, id string, extracted map[string]any) error
	// SetBotActive toggles conversations.bot_active (migration 018). Setting it
	// false mutes the bot for the conversation so the worker short-circuits future
	// turns (FR-6.1/6.3 handoff). Returns ErrNotFound when no such row exists.
	SetBotActive(ctx context.Context, id string, active bool) error

	// SetContact links a conversation to its contact (conversations.contact_id).
	// Required for GDPR erasure: the contact-keyed cascade reaches the conversation
	// + messages only via this link. Web conversations exist before the contact, so
	// the agent back-links on lead submission. Returns ErrNotFound for no such row.
	SetContact(ctx context.Context, id, contactID string) error

	// GetOrCreateByChannelPhone resolves the open conversation for a (channel,
	// phone) pair — the WhatsApp inbound path keys conversations by sender phone.
	// It returns the most recent open conversation whose linked contact has the
	// phone; when none exists it creates a contact carrying the phone and a new
	// conversation tagged with the given vertical/intent and linked to that
	// contact. Empty vertical/intent resolve to the defaults. The phone is stored,
	// never logged. All SQL is parameterized.
	GetOrCreateByChannelPhone(ctx context.Context, channel, phone, vertical, intent string) (*domain.Conversation, error)

	// ContactPhoneByConversation returns the phone of the conversation's linked
	// contact (conversations.contact_id), or ErrNotFound when the conversation,
	// its contact, or the phone is missing. It backs outbound WhatsApp delivery.
	ContactPhoneByConversation(ctx context.Context, conversationID string) (string, error)
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
	// CountByConversationRole returns how many messages a conversation has with the
	// given role (e.g. "user"). It is a cheap, parameterized COUNT that backs the
	// max-turns-per-conversation cost cap (SECURITY.md §1.1 D): the chat use-case
	// refuses a new user turn once the count reaches the configured cap, BEFORE any
	// paid LLM call. role must be non-empty.
	CountByConversationRole(ctx context.Context, conversationID, role string) (int, error)
}

type LeadRepo interface {
	Create(ctx context.Context, lead *domain.Lead) error
	GetByConversationID(ctx context.Context, convID string) (*domain.Lead, error)
	UpdateCompanyContact(ctx context.Context, leadID, companyID, contactID string) error
	// SetHumanFlags sets leads.needs_human and leads.bot_active (migration 016) on
	// the lead for a conversation, used on handoff. It is a no-op (nil error) when
	// no lead exists yet for the conversation — handoff can happen before a lead is
	// created, in which case only the conversation flag is flipped.
	SetHumanFlagsByConversation(ctx context.Context, convID string, needsHuman, botActive bool) error
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

// DocumentRepo persists the polymorphic document index (migration 021_documents).
// It records where an uploaded blob lives (gcs_key) and what it belongs to
// (owner_type, owner_id); the bytes themselves go through the FileStore port. The
// (owner_type, owner_id) pair is a SOFT polymorphic reference — no DB foreign key
// spans owner tables — so blob+row deletion is an explicit step on the GDPR
// erasure path, not a cascade. All statements are parameterized.
type DocumentRepo interface {
	// Create inserts one document row. The database assigns ID and CreatedAt,
	// which are written back onto the passed struct.
	Create(ctx context.Context, d *domain.Document) error
	// ListByOwner returns all documents for an owner, newest first.
	ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*domain.Document, error)
	// CountByOwnerKind returns the number of documents bound to (ownerType,
	// ownerID) with the given kind. It backs the PalletClearance seller-photo
	// blocking gate (count kind='photo' for a conversation before allowing a
	// listing submit).
	CountByOwnerKind(ctx context.Context, ownerType, ownerID, kind string) (int, error)
	// Reassign re-points every document of the given kind from one owner to
	// another, returning the number of rows moved. It is used by the seller
	// submit to attach the conversation-scoped photos to the durable listing
	// (from owner_type='conversation' to owner_type='listing') once the listing
	// is created. owner ids are soft polymorphic references (no DB FK); the move
	// is a parameterized UPDATE.
	Reassign(ctx context.Context, fromOwnerType, fromOwnerID, toOwnerType, toOwnerID, kind string) (int, error)
}

// ErrNotFound is returned by repository reads/mutations when the addressed row
// does not exist. The dashboard handlers map it to a 404 envelope.
var ErrNotFound = errors.New("not found")

type ContactRepo interface {
	Create(ctx context.Context, contact *domain.Contact) error
	Update(ctx context.Context, contact *domain.Contact) error
	// SetActiveConsent points contacts.consent_id at the given consent (the
	// deferred circular FK from migration 015 — the contact's *current* consent).
	// It is a no-op-safe parameterized UPDATE; a missing contact returns
	// ErrNotFound.
	SetActiveConsent(ctx context.Context, contactID, consentID string) error
	// FindIDByEmail resolves the most recent contact id carrying the given email,
	// or ErrNotFound when none exists. It backs the erasure-by-email convenience;
	// the email is matched case-insensitively and never logged.
	FindIDByEmail(ctx context.Context, email string) (string, error)
}

// ConsentRepo persists GDPR consent events (consents table, migration 015,
// invariant #7 / NFR-3). Consent is personal data: a row carries the IP at
// capture as proof and CASCADE-deletes with its contact. The full history lives
// here; the contact's active consent pointer is set via ContactRepo.SetActiveConsent.
// All statements are parameterized.
type ConsentRepo interface {
	// Create inserts one consent row, writing the assigned id/given_at/created_at
	// back onto c. An empty IP is stored as SQL NULL (the WhatsApp path).
	Create(ctx context.Context, c *domain.Consent) error
}

// ErasureRepo performs the transactional database half of the GDPR right-to-
// erasure cascade (SECURITY.md §7.4, DATA_MODEL_DDL §4). It owns its own
// transaction internally so no DB handle/tx type ever crosses the port boundary
// (hexagonal). The use-case (ErasureService) drives it, then deletes the returned
// blob keys via the FileStore and writes the final audit row.
//
// In one transaction Erase:
//   - collects every documents.gcs_key reachable from the contact (its leads and
//     their listings/sourcing_requests, and the contact's conversations) BEFORE
//     deleting anything;
//   - anonymizes (redacted=true, PII stripped from meta) the activity_logs rows
//     referencing the erased entities — it does NOT delete the audit trail;
//   - deletes those documents index rows (no DB FK cascade reaches them);
//   - DELETEs the contact, relying on the FK CASCADE chain (consents,
//     conversations→messages, leads→sourcing_requests|listings) and NOT touching
//     companies (public data; only SET NULL on links).
//
// It returns the collected blob keys (for out-of-band FileStore deletion) and a
// counts report. companies are never deleted here.
type ErasureRepo interface {
	// Erase runs the cascade for contactID. It returns ErrNotFound when no such
	// contact exists. blobKeys are the documents.gcs_key values to delete from the
	// FileStore after the transaction commits.
	Erase(ctx context.Context, contactID string) (report domain.ErasureReport, blobKeys []string, err error)
}

type SourcingRepo interface {
	Create(ctx context.Context, req *domain.SourcingRequest) error
	UpdateByLeadID(ctx context.Context, req *domain.SourcingRequest) error
}

// ListingRepo persists the PalletClearance SELLER typed request (migration 019).
// A listing is a sibling of the thin lead (invariant #5): one per lead. All SQL
// is parameterized. The seller-photo hard gate is enforced by the flow engine,
// not this repo.
type ListingRepo interface {
	// Create inserts one listing row, writing back the assigned ID/CreatedAt.
	Create(ctx context.Context, l *domain.Listing) error
	// UpsertByLead inserts the listing or, when one already exists for the lead
	// (UNIQUE lead_id), updates it in place. It backs the idempotent submit path
	// (a redelivered turn must not duplicate the listing).
	UpsertByLead(ctx context.Context, l *domain.Listing) error
}

// BuyerProfileRepo persists the PalletClearance BUYER standing-demand profile
// (migration 020). It is a sibling typed request of the thin lead (invariant
// #5) keyed by (company_id, vertical). All SQL is parameterized.
type BuyerProfileRepo interface {
	// Upsert inserts the buyer profile or updates the existing one for the same
	// (company_id, vertical), writing back the assigned ID/CreatedAt. It backs the
	// idempotent submit path.
	Upsert(ctx context.Context, p *domain.BuyerProfile) error
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
