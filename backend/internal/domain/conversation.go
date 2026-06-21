package domain

import "time"

type ConversationState string

const (
	StateGreeting   ConversationState = "greeting"
	StateQualifying ConversationState = "qualifying"
	StateVerifying  ConversationState = "verifying"
	StateConfirmed  ConversationState = "confirmed"
	StateFailed     ConversationState = "failed"
)

type Conversation struct {
	ID        string
	Channel   string
	State     ConversationState
	Extracted map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ToolCalls      []byte    `json:"tool_calls,omitempty"`
	ToolCallID     string    `json:"tool_call_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
