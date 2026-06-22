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

// SeenProviderMsg reports whether an inbound provider message id was already
// recorded. It reads through the partial-unique index messages_provider_msg_uq
// (migration 018). An empty providerMsgID is never "seen".
func (r *MessageRepo) SeenProviderMsg(ctx context.Context, providerMsgID string) (bool, error) {
	if providerMsgID == "" {
		return false, nil
	}
	var exists bool
	err := GetPool().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM messages WHERE provider_msg_id = $1
		)
	`, providerMsgID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ClaimProviderMsg atomically records the inbound user message with its provider
// id and reports whether this caller won the claim. The partial-unique index
// messages_provider_msg_uq makes the INSERT ... ON CONFLICT DO NOTHING the single
// source of truth: exactly one concurrent worker inserts the row (claimed=true);
// all others see zero rows affected (claimed=false) and must skip processing.
//
// Persisting the user message here means the agent core must not re-append it for
// provider-id-bearing turns; the worker passes the message through to the core
// purely as transient turn input.
func (r *MessageRepo) ClaimProviderMsg(ctx context.Context, conversationID, providerMsgID, content string) (bool, error) {
	if providerMsgID == "" {
		return false, nil
	}
	tag, err := GetPool().Exec(ctx, `
		INSERT INTO messages (conversation_id, role, content, provider_msg_id)
		VALUES ($1, 'user', $2, $3)
		ON CONFLICT (provider_msg_id) WHERE provider_msg_id IS NOT NULL
		DO NOTHING
	`, conversationID, content, providerMsgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// CountByConversationRole returns the number of messages for a conversation with
// the given role. It backs the max-turns-per-conversation cost cap (SECURITY.md
// §1.1): the chat use-case calls it before running a turn and refuses once the
// user-message count reaches the configured cap. The SQL is parameterized; an
// empty role yields 0 without a query.
func (r *MessageRepo) CountByConversationRole(ctx context.Context, conversationID, role string) (int, error) {
	if role == "" {
		return 0, nil
	}
	var n int
	err := GetPool().QueryRow(ctx, `
		SELECT count(*) FROM messages
		WHERE conversation_id = $1 AND role = $2
	`, conversationID, role).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
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
