package ports

import "context"

// TurnJob is the unit of asynchronous work the agent runtime processes: a single
// inbound message to be run as one agent turn for a conversation. It is
// JSON-serializable so it can travel through any transport (an in-process channel
// for the local adapter, or an HTTP push body for Cloud Tasks).
//
// ProviderMsgID is the channel-side message id (e.g. a WhatsApp wamid) used for
// idempotency/dedupe. It is empty for the current web flow, which carries no
// provider id; in that case no dedupe is applied and today's behavior is
// preserved.
type TurnJob struct {
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
	ProviderMsgID  string `json:"provider_msg_id,omitempty"`
}

// Queue durably hands an agent-turn job to a worker for asynchronous processing.
// The production adapter is Cloud Tasks (HTTP push to the worker endpoint); a
// local in-process adapter runs the registered handler in a goroutine so the demo
// works without external infrastructure.
//
// Selecting the implementation is a wiring concern driven by QUEUE_PROVIDER; the
// agent core and use-cases depend only on this interface.
type Queue interface {
	// Enqueue schedules a job for asynchronous processing. It returns once the job
	// has been durably accepted by the transport (committed to the queue / pushed),
	// not once the job has finished running.
	Enqueue(ctx context.Context, job TurnJob) error
}

// TurnProcessor runs a single agent turn for a job. It is the seam between the
// Queue (which delivers jobs) and the agent runtime (which executes them). Both
// the local queue adapter's handler and the worker HTTP endpoint invoke a
// TurnProcessor, so the processing path is identical regardless of transport.
type TurnProcessor interface {
	// Process runs the turn described by job: it takes the per-conversation lock,
	// applies provider-message-id idempotency, and drives the agent turn. A
	// non-nil error signals a retryable failure (the transport should retry); a
	// nil error means the job is done (including terminal outcomes that were
	// logged and swallowed).
	Process(ctx context.Context, job TurnJob) error
}
