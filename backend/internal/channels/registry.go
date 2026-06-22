// Package channels holds the per-surface reply adapters that satisfy the
// ports.Replier seam plus the registry that resolves one by channel name. It is
// the outbound half of the channel-agnostic agent: the async worker delivers a
// turn's reply through the Replier for the conversation's channel, so the agent
// core never knows which surface (web SSE, WhatsApp Cloud API, …) carries it.
//
// Adding a new channel = a new Replier adapter registered here; nothing in the
// agent core, flow engine, or worker changes (ARCHITECTURE_DETAIL §5c).
package channels

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// Registry maps a channel name to its Replier. It is constructed once at wiring
// time (the composition root) and resolved per turn by the worker.
type Registry struct {
	repliers map[string]ports.Replier
	fallback ports.Replier
}

var _ ports.ReplierRegistry = (*Registry)(nil)

// NewRegistry builds a Registry from a channel->Replier map. The map is copied so
// later mutation of the caller's map cannot affect routing. A nil/missing entry
// resolves to a no-op Replier via For, so an unknown or unconfigured channel
// still lets the turn complete.
func NewRegistry(repliers map[string]ports.Replier) *Registry {
	cp := make(map[string]ports.Replier, len(repliers))
	for name, r := range repliers {
		if r != nil {
			cp[name] = r
		}
	}
	return &Registry{repliers: cp, fallback: Noop{}}
}

// For returns the Replier registered for channel, or a no-op Replier when none is
// registered (never nil).
func (r *Registry) For(channel string) ports.Replier {
	if rep, ok := r.repliers[channel]; ok {
		return rep
	}
	return r.fallback
}

// Noop is a Replier that delivers nothing. It is the fallback for channels with
// no registered adapter so a turn never panics on reply delivery.
type Noop struct{}

var _ ports.Replier = Noop{}

// Typing is a no-op.
func (Noop) Typing(context.Context, *domain.Conversation) error { return nil }

// Deliver is a no-op.
func (Noop) Deliver(context.Context, *domain.Conversation, ports.Event) error { return nil }

// Error is a no-op.
func (Noop) Error(context.Context, *domain.Conversation, string) error { return nil }
