package usecases

import (
	"context"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ChatRequest is the inbound chat-turn payload. Vertical and Intent are optional:
// when omitted (the legacy widget sends neither) they default to the Angrosist
// buyer flow so existing behavior is preserved. When present they are validated
// against the supported lookup sets before a new conversation is created.
type ChatRequest struct {
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
	// Vertical selects the qualification path: "angrosist" | "palletclearance".
	// Empty defaults to angrosist. Only used when creating a new conversation.
	Vertical string `json:"vertical,omitempty"`
	// Intent selects buy vs sell within the vertical: "buy" | "sell". Empty
	// defaults to buy. Only used when creating a new conversation.
	Intent string `json:"intent,omitempty"`
}

// supportedVerticals / supportedIntents are the values the chat entrypoint accepts
// for a NEW conversation (Phase-1 PalletClearance + Angrosist; skalyou is P2 and
// not creatable from the widget yet). They mirror the verticals/intents lookups
// (migration 010) but bound what the public entrypoint will start.
var supportedVerticals = map[string]bool{
	domain.VerticalAngrosist:       true,
	domain.VerticalPalletClearance: true,
}

var supportedIntents = map[string]bool{
	domain.IntentBuy:  true,
	domain.IntentSell: true,
}

// ValidateVerticalIntent normalizes and validates an optional (vertical, intent)
// pair for a NEW conversation, returning the resolved values or an error when the
// pair is unsupported. The HTTP entrypoint calls it to reject bad input with a 400
// before invoking RunTurn (which would otherwise surface the same error as a 500).
func ValidateVerticalIntent(vertical, intent string) (string, string, error) {
	return validateVerticalIntent(vertical, intent)
}

// validateVerticalIntent normalizes and validates the optional vertical/intent.
// Empty values resolve to the defaults; a non-empty value not in the supported set
// is rejected so an attacker/bad client cannot create an unsupported flow.
func validateVerticalIntent(vertical, intent string) (string, string, error) {
	if vertical == "" {
		vertical = domain.DefaultVertical
	}
	if intent == "" {
		intent = domain.DefaultIntent
	}
	if !supportedVerticals[vertical] {
		return "", "", fmt.Errorf("unsupported vertical: %q", vertical)
	}
	if !supportedIntents[intent] {
		return "", "", fmt.Errorf("unsupported intent: %q", intent)
	}
	return vertical, intent, nil
}

type ChatResponse struct {
	ConversationID string         `json:"conversation_id"`
	Reply          string         `json:"reply"`
	State          string         `json:"state"`
	Extracted      map[string]any `json:"extracted"`
}

type ChatUseCase struct {
	convRepo ports.ConversationRepo
	runner   ports.AgentRunner
	locker   ports.Locker
	repliers ports.ReplierRegistry
}

// NewChatUseCase wires the synchronous chat path. The locker serializes turns
// per conversation: even though /api/chat stays synchronous for now (the async
// transport flip lands in Epic 2.3), concurrent requests for the same
// conversation run one at a time, preserving ordering and avoiding double-writes.
//
// repliers is the channel-agnostic reply seam: the use-case delivers typing/
// message/error events through the conversation's channel (web -> SSE broker) so
// a subscribed SSE client receives the turn live. The synchronous HTTP response
// is unchanged — delivery is additive and best-effort. repliers may be nil
// (delivery is then skipped) so tests can omit it. The /api/chat path only ever
// creates web conversations, so delivery always routes to the web channel here.
func NewChatUseCase(convRepo ports.ConversationRepo, runner ports.AgentRunner, locker ports.Locker, repliers ports.ReplierRegistry) *ChatUseCase {
	return &ChatUseCase{convRepo: convRepo, runner: runner, locker: locker, repliers: repliers}
}

// replierFor resolves the Replier for a conversation's channel, or nil when no
// registry was wired (delivery then becomes a no-op).
func (uc *ChatUseCase) replierFor(channel string) ports.Replier {
	if uc.repliers == nil {
		return nil
	}
	return uc.repliers.For(channel)
}

func (uc *ChatUseCase) RunTurn(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	convID := req.ConversationID
	if convID == "" {
		vertical, intent, err := validateVerticalIntent(req.Vertical, req.Intent)
		if err != nil {
			return nil, err
		}
		conv, err := uc.convRepo.CreateWith(ctx, "web", vertical, intent)
		if err != nil {
			return nil, err
		}
		convID = conv.ID
	}

	// /api/chat only ever serves the web widget, so reply delivery routes to the
	// web channel (the SSE broker). A minimal conversation carries the id/channel
	// for the Replier; the completed-turn delivery below uses the reloaded state.
	convRef := &domain.Conversation{ID: convID, Channel: "web"}
	replier := uc.replierFor(convRef.Channel)

	// Signal the typing indicator before the (potentially slow) turn work begins.
	if replier != nil {
		_ = replier.Typing(ctx, convRef)
	}

	var reply string
	err := uc.locker.WithConversationLock(ctx, convID, func(ctx context.Context) error {
		r, err := uc.runner.RunTurn(ctx, convID, req.Message)
		if err != nil {
			return err
		}
		reply = r
		return nil
	})
	if err != nil {
		// Deliver a safe error event (no internal detail / PII) for live clients,
		// then return the error to the HTTP caller unchanged.
		if replier != nil {
			_ = replier.Error(ctx, convRef, "agent error")
		}
		return nil, err
	}

	// Reload conversation to get current state and extracted fields
	conv, err := uc.convRepo.GetByID(ctx, convID)
	if err != nil {
		if replier != nil {
			_ = replier.Error(ctx, convRef, "agent error")
		}
		return nil, err
	}

	resp := &ChatResponse{
		ConversationID: convID,
		Reply:          reply,
		State:          string(conv.State),
		Extracted:      conv.Extracted,
	}

	// Deliver the completed turn so a subscribed SSE client receives it live. The
	// HTTP response below is identical to before — this is purely additive.
	if replier != nil {
		_ = replier.Deliver(ctx, conv, ports.Event{
			Type:      ports.EventMessage,
			Reply:     resp.Reply,
			State:     resp.State,
			Extracted: resp.Extracted,
		})
	}

	return resp, nil
}
