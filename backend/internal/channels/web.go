package channels

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ChannelWeb is the channel name for the embeddable web widget / hosted page.
const ChannelWeb = "web"

// Web is the ports.Replier for the web widget. It relays turn lifecycle events to
// the SSE broker, preserving the existing real-time stream behavior: a typing
// indicator before the turn, the completed message (with state + extracted) after
// it, and a safe error event on failure.
//
// It is a thin wrapper over ports.Broker so the broker (in-process today, Redis
// pub/sub in multi-instance prod) stays the SSE transport and the worker no
// longer publishes to it directly.
type Web struct {
	broker ports.Broker
}

var _ ports.Replier = (*Web)(nil)

// NewWeb constructs the web Replier over a broker. broker may be nil, in which
// case every method is a no-op (publishing is skipped) — matching the prior
// nil-safe behavior of the worker/use-case.
func NewWeb(broker ports.Broker) *Web { return &Web{broker: broker} }

func (w *Web) publish(convID string, ev ports.Event) {
	if w.broker != nil {
		w.broker.Publish(convID, ev)
	}
}

// Typing publishes a typing event to the conversation's SSE subscribers.
func (w *Web) Typing(_ context.Context, conv *domain.Conversation) error {
	w.publish(conv.ID, ports.Event{Type: ports.EventTyping})
	return nil
}

// Deliver publishes the completed assistant turn (reply + state + extracted) to
// the conversation's SSE subscribers.
func (w *Web) Deliver(_ context.Context, conv *domain.Conversation, ev ports.Event) error {
	if ev.Type == "" {
		ev.Type = ports.EventMessage
	}
	w.publish(conv.ID, ev)
	return nil
}

// Error publishes a safe error event to the conversation's SSE subscribers.
func (w *Web) Error(_ context.Context, conv *domain.Conversation, message string) error {
	w.publish(conv.ID, ports.Event{Type: ports.EventError, Error: message})
	return nil
}
