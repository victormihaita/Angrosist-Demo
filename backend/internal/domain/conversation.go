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

// Vertical and Intent codes mirror the verticals/intents lookup tables
// (migration 010). They key the flow engine's per-flow required-field set and
// system prompt (AI_AGENT_SPEC §1/§2). The defaults below preserve the original
// single-vertical demo behavior when a conversation omits them.
const (
	VerticalAngrosist       = "angrosist"
	VerticalPalletClearance = "palletclearance"

	IntentBuy  = "buy"
	IntentSell = "sell"

	// DefaultVertical / DefaultIntent are applied when a conversation is created
	// without an explicit vertical/intent, keeping the existing web widget working
	// unchanged (it sends neither). NULL columns in the DB resolve to these in code
	// rather than via a destructive DB default.
	DefaultVertical = VerticalAngrosist
	DefaultIntent   = IntentBuy
)

type Conversation struct {
	ID        string
	Channel   string
	State     ConversationState
	Extracted map[string]any
	// BotActive mirrors conversations.bot_active (migration 018). When false the
	// bot is muted (handed off to a human) and the worker short-circuits the turn.
	// Defaults to true for new conversations.
	BotActive bool
	// Language is the detected/declared conversation language ("ro" | "en"), used
	// to localize transactional email. Empty when unknown.
	Language string
	// Vertical and Intent select the active flow (AI_AGENT_SPEC §1/§2). They are
	// resolved to DefaultVertical/DefaultIntent in code when the persisted columns
	// are NULL (legacy demo conversations), so the Angrosist buyer flow stays the
	// default. Values come from the verticals/intents lookups (migration 010).
	Vertical  string
	Intent    string
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
