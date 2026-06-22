package ports

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
)

// Replier delivers an agent turn's lifecycle events back to the surface a
// conversation lives on. It is the channel-agnostic reply seam: the async worker
// (and the synchronous chat use-case) emit typing/message/error events through a
// Replier resolved by the conversation's channel, instead of talking to a
// specific transport (the SSE broker, the WhatsApp Cloud API, …) directly.
//
// There is one Replier adapter per channel:
//
//   - web      — wraps the ports.Broker publish so the existing SSE stream keeps
//     working (typing + message + error events).
//   - whatsapp — sends the reply text back through the WhatsApp Cloud API sender
//     to the conversation's contact phone; typing/error events are no-ops (there
//     is no live "typing" surface on WhatsApp and an internal error must not leak
//     to the end user).
//
// Adding a new channel = a new Replier adapter registered under its name; the
// agent core, flow engine, and worker are unchanged (the "add a channel" swap
// recipe, ARCHITECTURE_DETAIL §5c).
type Replier interface {
	// Typing signals that the agent is working on a reply (e.g. a typing
	// indicator). Channels without such a surface implement it as a no-op.
	Typing(ctx context.Context, conv *domain.Conversation) error

	// Deliver sends a completed assistant turn (ev.Reply) back through the
	// channel. State/Extracted on the event are carried for surfaces (web SSE)
	// that relay them; text-only channels (WhatsApp) use only ev.Reply.
	Deliver(ctx context.Context, conv *domain.Conversation, ev Event) error

	// Error reports a failed turn to live clients with a safe (no-PII, no
	// internal detail) message. Channels that must not surface internal errors to
	// the end user (WhatsApp) implement it as a no-op.
	Error(ctx context.Context, conv *domain.Conversation, message string) error
}

// ReplierRegistry resolves the Replier for a conversation's channel. It is the
// single place channel selection happens; callers never branch on the channel
// name themselves.
type ReplierRegistry interface {
	// For returns the Replier registered under channel. When no adapter is
	// registered for the channel it returns a no-op Replier (never nil) so a
	// turn for an unknown/unconfigured channel still completes without panicking.
	For(channel string) Replier
}
