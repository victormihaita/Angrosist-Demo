// Package broker holds adapters that satisfy the ports.Broker pub/sub seam for
// real-time conversation events. It ships an in-process adapter for the
// single-instance demo/dev runtime.
//
// IMPORTANT: the in-process adapter only fans out to subscribers on the SAME
// process. It is correct for local dev and a single Cloud Run instance. A
// multi-instance production deployment (where the SSE connection may land on a
// different instance than the worker that ran the turn) MUST swap in a Redis
// pub/sub adapter behind the same ports.Broker interface — that is a wiring
// change only; no caller changes. Do not add Redis here until that milestone.
package broker

import (
	"sync"

	"github.com/angrosist/demo/internal/ports"
)

var _ ports.Broker = (*InProcess)(nil)

// subBufferSize bounds each subscriber's channel. Turn events are infrequent
// (typing, message, error per turn), so a small buffer absorbs a brief slow
// reader; beyond that Publish drops rather than blocking the turn.
const subBufferSize = 16

// InProcess is a single-process ports.Broker: a per-conversation registry of
// subscriber channels with concurrency-safe registration and non-blocking
// publish. Suitable for the demo and a single instance only (see package doc).
type InProcess struct {
	mu sync.Mutex
	// subs maps conversationID -> set of subscriber channels. A nil/empty entry is
	// pruned on unsubscribe so the map does not grow unbounded.
	subs map[string]map[chan ports.Event]struct{}
}

// NewInProcess constructs an empty in-process broker.
func NewInProcess() *InProcess {
	return &InProcess{subs: make(map[string]map[chan ports.Event]struct{})}
}

// Subscribe registers a new subscriber for conversationID and returns its
// receive channel plus an idempotent unsubscribe function. Unsubscribe removes
// the channel from the registry and closes it; calling it more than once is safe.
func (b *InProcess) Subscribe(conversationID string) (<-chan ports.Event, func()) {
	ch := make(chan ports.Event, subBufferSize)

	b.mu.Lock()
	set := b.subs[conversationID]
	if set == nil {
		set = make(map[chan ports.Event]struct{})
		b.subs[conversationID] = set
	}
	set[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			if set := b.subs[conversationID]; set != nil {
				delete(set, ch)
				if len(set) == 0 {
					delete(b.subs, conversationID)
				}
			}
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, unsubscribe
}

// Publish fans ev out to every current subscriber of conversationID without
// blocking. A subscriber whose buffer is full is skipped (the event is dropped
// for that consumer) so a slow client can never stall the agent turn. Publishing
// to a conversation with no subscribers is a no-op.
func (b *InProcess) Publish(conversationID string, ev ports.Event) {
	b.mu.Lock()
	set := b.subs[conversationID]
	// Snapshot the channels under the lock, then send outside it so a send never
	// holds the registry lock.
	chans := make([]chan ports.Event, 0, len(set))
	for ch := range set {
		chans = append(chans, ch)
	}
	b.mu.Unlock()

	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			// Full buffer (slow subscriber): drop rather than block the turn.
		}
	}
}
