package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
