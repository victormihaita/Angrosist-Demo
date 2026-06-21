package postgres

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
)

type MessageRepo struct{}

func NewMessageRepo() *MessageRepo { return &MessageRepo{} }

func (r *MessageRepo) Append(ctx context.Context, msg *domain.Message) error {
	_, err := GetPool().Exec(ctx, `
		INSERT INTO messages (conversation_id, role, content, tool_calls, tool_call_id)
		VALUES ($1, $2, $3, $4, $5)
	`, msg.ConversationID, msg.Role, msg.Content, nullBytes(msg.ToolCalls), nullStr(msg.ToolCallID))
	return err
}

func (r *MessageRepo) ListByConversation(ctx context.Context, conversationID string) ([]*domain.Message, error) {
	rows, err := GetPool().Query(ctx, `
		SELECT id, conversation_id, role, content, tool_calls, tool_call_id, created_at
		FROM messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*domain.Message
	for rows.Next() {
		var m domain.Message
		var toolCalls []byte
		var toolCallID *string
		var content *string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &content, &toolCalls, &toolCallID, &m.CreatedAt); err != nil {
			return nil, err
		}
		if content != nil {
			m.Content = *content
		}
		m.ToolCalls = toolCalls
		if toolCallID != nil {
			m.ToolCallID = *toolCallID
		}
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
