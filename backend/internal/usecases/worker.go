package usecases

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/angrosist/demo/internal/ports"
)

// AsyncRunner is the agent-turn entrypoint the worker drives. The agent core
// satisfies it.
//
//   - RunTurn appends the inbound user message itself (the no-provider-id web
//     path).
//   - RunTurnPersisted runs a turn whose inbound user message was already
//     persisted by the idempotency claim (the provider-id channel path).
type AsyncRunner interface {
	RunTurn(ctx context.Context, conversationID, userMessage string) (reply string, err error)
	RunTurnPersisted(ctx context.Context, conversationID, userMessage string) (reply string, err error)
}

// TurnWorker is the asynchronous turn processor. Given a ports.TurnJob it takes
// the per-conversation lock, applies provider-message-id idempotency, then runs
// the agent turn. It implements ports.TurnProcessor and is invoked identically by
// the local queue adapter's handler and the worker HTTP endpoint.
//
// Error classification: TurnWorker returns a non-nil error only for retryable
// failures (the transport should retry). Terminal failures and business outcomes
// are logged and swallowed (nil) so the transport does not retry. The agent core
// already turns business outcomes (company not found/unavailable) into tool
// results rather than errors, so an error bubbling out of RunTurnPersisted is
// treated as retryable by default.
type TurnWorker struct {
	runner   AsyncRunner
	locker   ports.Locker
	msgRepo  ports.MessageRepo
	convRepo ports.ConversationRepo
	broker   ports.Broker
}

var _ ports.TurnProcessor = (*TurnWorker)(nil)

// NewTurnWorker wires the async turn processor.
//
// convRepo and broker power live delivery: after a turn the worker reloads the
// conversation and publishes a message event so a subscribed SSE client receives
// the async reply (the same events the synchronous /api/chat path emits). Both
// may be nil — publishing is then skipped and the worker behaves exactly as
// before, keeping existing callers/tests working.
func NewTurnWorker(runner AsyncRunner, locker ports.Locker, msgRepo ports.MessageRepo, convRepo ports.ConversationRepo, broker ports.Broker) *TurnWorker {
	return &TurnWorker{runner: runner, locker: locker, msgRepo: msgRepo, convRepo: convRepo, broker: broker}
}

// publish is a nil-safe Broker.Publish so a missing broker is a no-op.
func (w *TurnWorker) publish(convID string, ev ports.Event) {
	if w.broker != nil {
		w.broker.Publish(convID, ev)
	}
}

// publishReply reloads the conversation and publishes a completed-turn message
// event. Best-effort: a reload failure is logged (no PII) and does not affect the
// turn outcome.
func (w *TurnWorker) publishReply(ctx context.Context, convID, reply string) {
	if w.broker == nil {
		return
	}
	ev := ports.Event{Type: ports.EventMessage, Reply: reply}
	if w.convRepo != nil {
		if conv, err := w.convRepo.GetByID(ctx, convID); err != nil {
			log.Printf("worker: load conversation for broker publish conversation=%s: %v", convID, err)
		} else {
			ev.State = string(conv.State)
			ev.Extracted = conv.Extracted
		}
	}
	w.publish(convID, ev)
}

// Process runs one job. Concurrency for the same conversation is serialized by
// the lock; idempotency is enforced inside the lock via the provider message id.
func (w *TurnWorker) Process(ctx context.Context, job ports.TurnJob) error {
	if job.ConversationID == "" {
		// Malformed job: nothing to lock on. Terminal — log and ack.
		log.Printf("worker: dropping job with empty conversation_id provider_msg_id=%q", job.ProviderMsgID)
		return nil
	}

	return w.locker.WithConversationLock(ctx, job.ConversationID, func(ctx context.Context) error {
		// Idempotency (inside the lock — the authoritative check). For jobs with a
		// provider message id, atomically claim it; if a prior turn already
		// claimed it, this is a redelivery — skip processing. Web turns carry no
		// provider id and are never deduped (today's behavior preserved).
		if job.ProviderMsgID != "" {
			claimed, err := w.msgRepo.ClaimProviderMsg(ctx, job.ConversationID, job.ProviderMsgID, job.Message)
			if err != nil {
				return fmt.Errorf("worker: claim provider msg %s: %w", job.ProviderMsgID, err)
			}
			if !claimed {
				log.Printf("worker: skipping duplicate provider_msg_id=%q conversation=%s", job.ProviderMsgID, job.ConversationID)
				return nil
			}
			// Signal typing before the turn work (only for messages we will process).
			w.publish(job.ConversationID, ports.Event{Type: ports.EventTyping})
			// The user message is now persisted; run the turn without re-appending.
			reply, err := w.runner.RunTurnPersisted(ctx, job.ConversationID, job.Message)
			if err != nil {
				w.publish(job.ConversationID, ports.Event{Type: ports.EventError, Error: "agent error"})
				return classify(fmt.Errorf("worker: run turn (provider_msg_id=%q): %w", job.ProviderMsgID, err))
			}
			w.publishReply(ctx, job.ConversationID, reply)
			return nil
		}

		// No provider id (web flow): persist+run via the standard path. The agent
		// core appends the user message itself in this branch.
		w.publish(job.ConversationID, ports.Event{Type: ports.EventTyping})
		reply, err := w.runner.RunTurn(ctx, job.ConversationID, job.Message)
		if err != nil {
			w.publish(job.ConversationID, ports.Event{Type: ports.EventError, Error: "agent error"})
			return classify(fmt.Errorf("worker: run turn (conversation=%s): %w", job.ConversationID, err))
		}
		w.publishReply(ctx, job.ConversationID, reply)
		return nil
	})
}

// errTerminal marks an error that must not be retried by the transport.
var errTerminal = errors.New("terminal")

// TerminalError wraps err so the worker swallows it (acks) instead of retrying.
// Adapters/use-cases can use this to flag permanent failures explicitly.
func TerminalError(err error) error { return fmt.Errorf("%w: %w", errTerminal, err) }

// classify decides whether an agent-turn error should be retried. Terminal
// errors (explicitly wrapped) are logged and swallowed (return nil); everything
// else is treated as retryable and returned so the transport retries with
// backoff.
func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errTerminal) {
		log.Printf("worker: terminal error, not retrying: %v", err)
		return nil
	}
	return err
}
