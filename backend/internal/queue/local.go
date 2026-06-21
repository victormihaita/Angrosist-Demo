// Package queue holds the adapters that satisfy the ports.Queue seam: a local
// in-process queue for the demo/single-instance runtime and a Cloud Tasks
// HTTP-push queue for production. Selection is a wiring concern driven by
// QUEUE_PROVIDER; nothing here is referenced by the agent core or use-cases.
package queue

import (
	"context"
	"log"

	"github.com/angrosist/demo/internal/ports"
)

var _ ports.Queue = (*Local)(nil)

// Local is an in-process ports.Queue: Enqueue runs the registered TurnProcessor
// in a goroutine, simulating asynchronous delivery without external
// infrastructure so `docker compose` and local dev work end-to-end. It is the
// default (QUEUE_PROVIDER=local).
//
// Delivery is at-most-once and process-local: a crash loses in-flight jobs. That
// is acceptable for the demo; production uses the Cloud Tasks adapter for
// durability and retries.
type Local struct {
	processor ports.TurnProcessor
}

// NewLocal constructs the in-process queue bound to the given processor.
func NewLocal(processor ports.TurnProcessor) *Local {
	return &Local{processor: processor}
}

// Enqueue dispatches the job to the processor on a new goroutine and returns
// immediately, mirroring the ack-fast-then-process contract of the async runtime.
// The goroutine derives a context detached from the request's cancellation so the
// turn is not cancelled when the enqueuing handler returns.
func (q *Local) Enqueue(ctx context.Context, job ports.TurnJob) error {
	jobCtx := context.WithoutCancel(ctx)
	go func() {
		if err := q.processor.Process(jobCtx, job); err != nil {
			// Local queue has no retry transport; a returned (retryable) error is
			// logged for visibility. Durable retry is the Cloud Tasks adapter's job.
			log.Printf("queue(local): process failed conversation=%s provider_msg_id=%q: %v",
				job.ConversationID, job.ProviderMsgID, err)
		}
	}()
	return nil
}
