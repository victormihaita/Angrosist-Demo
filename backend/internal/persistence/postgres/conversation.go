package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

type ConversationRepo struct{}

func NewConversationRepo() *ConversationRepo { return &ConversationRepo{} }

func (r *ConversationRepo) Create(ctx context.Context, channel string) (*domain.Conversation, error) {
	return r.CreateWith(ctx, channel, "", "")
}

// CreateWith creates a conversation tagged with a vertical and intent. Empty
// values default to domain.DefaultVertical/DefaultIntent so the legacy widget
// (which sends neither) keeps the Angrosist-buyer flow.
func (r *ConversationRepo) CreateWith(ctx context.Context, channel, vertical, intent string) (*domain.Conversation, error) {
	if vertical == "" {
		vertical = domain.DefaultVertical
	}
	if intent == "" {
		intent = domain.DefaultIntent
	}
	row := GetPool().QueryRow(ctx, `
		INSERT INTO conversations (channel, state, extracted, vertical, intent)
		VALUES ($1, 'greeting', '{}', $2, $3)
		RETURNING id, channel, state, extracted, bot_active, COALESCE(language, ''),
		          COALESCE(vertical, ''), COALESCE(intent, ''), created_at, updated_at
	`, channel, vertical, intent)
	return scanConversation(row)
}

func (r *ConversationRepo) GetByID(ctx context.Context, id string) (*domain.Conversation, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT id, channel, state, extracted, bot_active, COALESCE(language, ''),
		       COALESCE(vertical, ''), COALESCE(intent, ''), created_at, updated_at
		FROM conversations WHERE id = $1
	`, id)
	return scanConversation(row)
}

func (r *ConversationRepo) UpdateState(ctx context.Context, id string, state domain.ConversationState) error {
	_, err := GetPool().Exec(ctx, `
		UPDATE conversations SET state = $1, updated_at = NOW() WHERE id = $2
	`, string(state), id)
	return err
}

func (r *ConversationRepo) UpdateExtracted(ctx context.Context, id string, extracted map[string]any) error {
	b, err := json.Marshal(extracted)
	if err != nil {
		return err
	}
	_, err = GetPool().Exec(ctx, `
		UPDATE conversations SET extracted = $1, updated_at = NOW() WHERE id = $2
	`, b, id)
	return err
}

// SetBotActive toggles conversations.bot_active (migration 018). Used on handoff
// to mute the bot for the conversation. Returns ErrNotFound when no such row
// exists.
func (r *ConversationRepo) SetBotActive(ctx context.Context, id string, active bool) error {
	tag, err := GetPool().Exec(ctx, `
		UPDATE conversations SET bot_active = $1, updated_at = NOW() WHERE id = $2::uuid
	`, active, id)
	if err != nil {
		return fmt.Errorf("set bot_active: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// SetContact links a conversation to its contact (conversations.contact_id).
// Essential for GDPR right-to-erasure: the contact-keyed cascade reaches the
// conversation (and its messages) only through this link. Web conversations are
// created before the contact exists, so the agent back-links on lead submission.
func (r *ConversationRepo) SetContact(ctx context.Context, id, contactID string) error {
	tag, err := GetPool().Exec(ctx, `
		UPDATE conversations SET contact_id = $1::uuid, updated_at = NOW() WHERE id = $2::uuid
	`, contactID, id)
	if err != nil {
		return fmt.Errorf("set contact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// GetOrCreateByChannelPhone resolves (or creates) the open conversation for a
// (channel, phone) pair. The WhatsApp inbound path keys conversations by sender
// phone: it first looks for the most recent non-failed conversation on the
// channel whose linked contact carries the phone; failing that it creates a
// contact (carrying the phone) and a new conversation linked to it, tagged with
// the resolved vertical/intent. The whole get-or-create runs in one transaction
// so a concurrent first inbound from the same number cannot create two
// conversations partially. All statements are parameterized; the phone is stored,
// never logged.
func (r *ConversationRepo) GetOrCreateByChannelPhone(ctx context.Context, channel, phone, vertical, intent string) (*domain.Conversation, error) {
	if vertical == "" {
		vertical = domain.DefaultVertical
	}
	if intent == "" {
		intent = domain.DefaultIntent
	}

	tx, err := GetPool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get-or-create conversation: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1) Existing open conversation for this channel + phone (newest first).
	row := tx.QueryRow(ctx, `
		SELECT c.id, c.channel, c.state, c.extracted, c.bot_active, COALESCE(c.language, ''),
		       COALESCE(c.vertical, ''), COALESCE(c.intent, ''), c.created_at, c.updated_at
		FROM conversations c
		JOIN contacts ct ON ct.id = c.contact_id
		WHERE c.channel = $1 AND ct.phone = $2 AND c.state <> 'failed'
		ORDER BY c.created_at DESC
		LIMIT 1
	`, channel, phone)
	conv, err := scanConversation(row)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("get-or-create conversation: commit: %w", cerr)
		}
		return conv, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get-or-create conversation: lookup: %w", err)
	}

	// 2) None found: create the contact carrying the phone, then the conversation.
	var contactID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO contacts (phone) VALUES ($1) RETURNING id
	`, phone).Scan(&contactID); err != nil {
		return nil, fmt.Errorf("get-or-create conversation: create contact: %w", err)
	}

	// GDPR consent capture at WhatsApp FIRST CONTACT (SECURITY.md §7.1): record a
	// consents row (channel='whatsapp', no HTTP request IP -> NULL) and point the
	// new contact's active-consent pointer at it, all in this transaction so the
	// contact never exists without its consent. text_version comes from env
	// (CONSENT_TEXT_VERSION) via the package-level default resolver; the audit
	// "consent.captured" row is written by the caller after commit is not needed
	// here — the webhook path enqueues a turn, and the durable consent proof is the
	// row itself. The phone is stored, never logged.
	var consentID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO consents (contact_id, text_version, channel, ip)
		VALUES ($1, $2, 'whatsapp', NULL)
		RETURNING id
	`, contactID, consentTextVersion()).Scan(&consentID); err != nil {
		return nil, fmt.Errorf("get-or-create conversation: capture consent: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contacts SET consent_id = $2 WHERE id = $1
	`, contactID, consentID); err != nil {
		return nil, fmt.Errorf("get-or-create conversation: set active consent: %w", err)
	}

	row = tx.QueryRow(ctx, `
		INSERT INTO conversations (channel, state, extracted, vertical, intent, contact_id)
		VALUES ($1, 'greeting', '{}', $2, $3, $4)
		RETURNING id, channel, state, extracted, bot_active, COALESCE(language, ''),
		          COALESCE(vertical, ''), COALESCE(intent, ''), created_at, updated_at
	`, channel, vertical, intent, contactID)
	conv, err = scanConversation(row)
	if err != nil {
		return nil, fmt.Errorf("get-or-create conversation: create conversation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("get-or-create conversation: commit: %w", err)
	}
	return conv, nil
}

// ContactPhoneByConversation returns the phone of the conversation's linked
// contact, or ports.ErrNotFound when the conversation, its contact, or the phone
// is missing. It backs outbound WhatsApp delivery.
func (r *ConversationRepo) ContactPhoneByConversation(ctx context.Context, conversationID string) (string, error) {
	var phone *string
	err := GetPool().QueryRow(ctx, `
		SELECT ct.phone
		FROM conversations c
		JOIN contacts ct ON ct.id = c.contact_id
		WHERE c.id = $1
	`, conversationID).Scan(&phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ports.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("contact phone by conversation %s: %w", conversationID, err)
	}
	if phone == nil || *phone == "" {
		return "", ports.ErrNotFound
	}
	return *phone, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanConversation(row scannable) (*domain.Conversation, error) {
	var c domain.Conversation
	var rawExtracted []byte
	var updatedAt time.Time

	err := row.Scan(&c.ID, &c.Channel, &c.State, &rawExtracted, &c.BotActive, &c.Language,
		&c.Vertical, &c.Intent, &c.CreatedAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.UpdatedAt = updatedAt
	// Resolve NULL/empty taxonomy columns (legacy demo rows) to the defaults so the
	// flow engine always sees a concrete (vertical, intent).
	if c.Vertical == "" {
		c.Vertical = domain.DefaultVertical
	}
	if c.Intent == "" {
		c.Intent = domain.DefaultIntent
	}
	if len(rawExtracted) > 0 {
		json.Unmarshal(rawExtracted, &c.Extracted)
	}
	if c.Extracted == nil {
		c.Extracted = make(map[string]any)
	}
	return &c, nil
}
