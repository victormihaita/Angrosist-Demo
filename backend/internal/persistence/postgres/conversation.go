package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/angrosist/demo/internal/domain"
)

type ConversationRepo struct{}

func NewConversationRepo() *ConversationRepo { return &ConversationRepo{} }

func (r *ConversationRepo) Create(ctx context.Context, channel string) (*domain.Conversation, error) {
	row := GetPool().QueryRow(ctx, `
		INSERT INTO conversations (channel, state, extracted)
		VALUES ($1, 'greeting', '{}')
		RETURNING id, channel, state, extracted, created_at, updated_at
	`, channel)
	return scanConversation(row)
}

func (r *ConversationRepo) GetByID(ctx context.Context, id string) (*domain.Conversation, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT id, channel, state, extracted, created_at, updated_at
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

type scannable interface {
	Scan(dest ...any) error
}

func scanConversation(row scannable) (*domain.Conversation, error) {
	var c domain.Conversation
	var rawExtracted []byte
	var updatedAt time.Time

	err := row.Scan(&c.ID, &c.Channel, &c.State, &rawExtracted, &c.CreatedAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.UpdatedAt = updatedAt
	if len(rawExtracted) > 0 {
		json.Unmarshal(rawExtracted, &c.Extracted)
	}
	if c.Extracted == nil {
		c.Extracted = make(map[string]any)
	}
	return &c, nil
}
