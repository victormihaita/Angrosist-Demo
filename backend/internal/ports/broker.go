package ports

// Event is a single real-time conversation event delivered to subscribers (e.g.
// the web widget's SSE stream). It is JSON-serializable so any Broker adapter
// (in-process today, Redis pub/sub in multi-instance prod) can carry it across a
// transport unchanged.
//
// Type is the event kind; the SSE layer maps it to the SSE `event:` name. The
// defined kinds are:
//
//   - EventTyping  — the agent is working on a reply (typing indicator on).
//   - EventMessage — a completed assistant turn; carries Reply, State, Extracted.
//   - EventState   — a flow-state change without a reply (optional).
//   - EventError   — a turn failed; carries a safe Error message (no PII).
//
// Fields are populated per kind: a typing event sets only Type; a message event
// sets Reply/State/Extracted; an error event sets Error.
type Event struct {
	Type      string         `json:"type"`
	Reply     string         `json:"reply,omitempty"`
	State     string         `json:"state,omitempty"`
	Extracted map[string]any `json:"extracted,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// Event type constants. Keep these in sync with the SSE contract in
// docs/specs/API_CONTRACT.md §5.3.
const (
	EventTyping  = "typing"
	EventMessage = "message"
	EventState   = "state"
	EventError   = "error"
)

// Broker is the pub/sub seam for real-time conversation events. Use-cases and the
// async worker Publish turn events; transport handlers (SSE/WS) Subscribe to a
// conversation and relay events to the connected client.
//
// It is a port: the agent core and domain never reference it; only the
// use-case/worker layer (which publishes) and the transport layer (which
// subscribes) depend on this interface. The single-instance in-process adapter
// lives in internal/broker; a multi-instance deployment swaps in a Redis pub/sub
// adapter behind the same interface with zero changes to callers.
type Broker interface {
	// Subscribe registers a subscriber for the given conversation and returns a
	// receive-only channel of events plus an unsubscribe function. The caller MUST
	// invoke the unsubscribe function when done (e.g. on client disconnect) to
	// release the subscription; after unsubscribe no further events are delivered
	// and the channel is closed.
	Subscribe(conversationID string) (<-chan Event, func())

	// Publish delivers ev to every current subscriber of conversationID. It never
	// blocks the caller: if a subscriber's buffer is full (a slow consumer), the
	// event is dropped for that subscriber rather than stalling the turn. Publish
	// to a conversation with no subscribers is a no-op.
	Publish(conversationID string, ev Event)
}
