package usecases

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/angrosist/demo/internal/domain"
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
// Reply delivery is channel-agnostic: after a turn the worker resolves the
// conversation's channel through the ports.ReplierRegistry and delivers the turn
// lifecycle events (typing/message/error) through that channel's ports.Replier.
// The web channel publishes to the SSE broker (unchanged behavior); the WhatsApp
// channel sends the reply via the Cloud API. The agent core is untouched.
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
	repliers ports.ReplierRegistry
}

var _ ports.TurnProcessor = (*TurnWorker)(nil)

// NewTurnWorker wires the async turn processor.
//
// convRepo and repliers power live delivery: after a turn the worker reloads the
// conversation and delivers the reply through the channel resolved by
// conv.Channel (web -> SSE broker, whatsapp -> Cloud API). Both may be nil —
// delivery is then skipped and the worker behaves exactly as before, keeping
// existing callers/tests working.
func NewTurnWorker(runner AsyncRunner, locker ports.Locker, msgRepo ports.MessageRepo, convRepo ports.ConversationRepo, repliers ports.ReplierRegistry) *TurnWorker {
	return &TurnWorker{runner: runner, locker: locker, msgRepo: msgRepo, convRepo: convRepo, repliers: repliers}
}

// conversation loads the conversation for delivery routing. It returns nil when
// the registry/convRepo are absent or the load fails (best-effort: a load error
// is logged with no PII and never affects the turn outcome).
func (w *TurnWorker) conversation(ctx context.Context, convID string) *domain.Conversation {
	if w.repliers == nil || w.convRepo == nil {
		return nil
	}
	conv, err := w.convRepo.GetByID(ctx, convID)
	if err != nil {
		log.Printf("worker: load conversation for reply delivery conversation=%s: %v", convID, err)
		return nil
	}
	return conv
}

// typing signals the typing indicator through the conversation's channel.
// Best-effort and nil-safe.
func (w *TurnWorker) typing(ctx context.Context, conv *domain.Conversation) {
	if conv == nil {
		return
	}
	if err := w.repliers.For(conv.Channel).Typing(ctx, conv); err != nil {
		log.Printf("worker: typing delivery conversation=%s: %v", conv.ID, err)
	}
}

// deliver sends the completed turn reply through the conversation's channel.
// Best-effort and nil-safe (a delivery error is logged with no PII; it does not
// fail the turn — the turn already committed).
func (w *TurnWorker) deliver(ctx context.Context, conv *domain.Conversation, reply string) {
	if conv == nil {
		return
	}
	ev := ports.Event{Type: ports.EventMessage, Reply: reply, State: string(conv.State), Extracted: conv.Extracted}
	if err := w.repliers.For(conv.Channel).Deliver(ctx, conv, ev); err != nil {
		log.Printf("worker: reply delivery conversation=%s: %v", conv.ID, err)
	}
}

// deliverError reports a failed turn to live clients through the conversation's
// channel. Best-effort and nil-safe.
func (w *TurnWorker) deliverError(ctx context.Context, conv *domain.Conversation, message string) {
	if conv == nil {
		return
	}
	if err := w.repliers.For(conv.Channel).Error(ctx, conv, message); err != nil {
		log.Printf("worker: error delivery conversation=%s: %v", conv.ID, err)
	}
}

// Note on the max-turns cap: the synchronous /api/chat path enforces the
// MAX_TURNS_PER_CONVERSATION cost cap in ChatUseCase before any LLM call. The
// WhatsApp/worker path is not capped here yet — it carries channel-side abuse
// controls (Meta rate limits) and would need the MessageRepo count threaded in;
// when WhatsApp volume warrants it, add the same CountByConversationRole check at
// the top of the locked section before RunTurnPersisted.
//
// Process runs one job. Concurrency for the same conversation is serialized by
// the lock; idempotency is enforced inside the lock via the provider message id.
func (w *TurnWorker) Process(ctx context.Context, job ports.TurnJob) error {
	if job.ConversationID == "" {
		// Malformed job: nothing to lock on. Terminal — log and ack.
		log.Printf("worker: dropping job with empty conversation_id provider_msg_id=%q", job.ProviderMsgID)
		return nil
	}

	return w.locker.WithConversationLock(ctx, job.ConversationID, func(ctx context.Context) error {
		// Resolve the conversation once for typing + reply routing by channel.
		conv := w.conversation(ctx, job.ConversationID)

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
			w.typing(ctx, conv)
			// The user message is now persisted; run the turn without re-appending.
			reply, err := w.runner.RunTurnPersisted(ctx, job.ConversationID, job.Message)
			if err != nil {
				w.deliverError(ctx, conv, "agent error")
				return classify(fmt.Errorf("worker: run turn (provider_msg_id=%q): %w", job.ProviderMsgID, err))
			}
			// Reload to capture the post-turn state/extracted for delivery.
			w.deliver(ctx, w.conversation(ctx, job.ConversationID), reply)
			return nil
		}

		// No provider id (web flow): persist+run via the standard path. The agent
		// core appends the user message itself in this branch.
		w.typing(ctx, conv)
		reply, err := w.runner.RunTurn(ctx, job.ConversationID, job.Message)
		if err != nil {
			w.deliverError(ctx, conv, "agent error")
			return classify(fmt.Errorf("worker: run turn (conversation=%s): %w", job.ConversationID, err))
		}
		w.deliver(ctx, w.conversation(ctx, job.ConversationID), reply)
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
