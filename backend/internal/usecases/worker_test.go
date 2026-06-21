package usecases

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/lock"
	"github.com/angrosist/demo/internal/ports"
)

func newJob(convID, msg string) ports.TurnJob {
	return ports.TurnJob{ConversationID: convID, Message: msg}
}

func newProviderJob(convID, providerMsgID string) ports.TurnJob {
	return ports.TurnJob{ConversationID: convID, Message: "msg", ProviderMsgID: providerMsgID}
}

// --- fakes -------------------------------------------------------------------

// fakeRunner counts how many times each entrypoint ran and can inject an error.
type fakeRunner struct {
	mu          sync.Mutex
	persistRuns int
	syncRuns    int
	err         error
}

func (f *fakeRunner) RunTurn(ctx context.Context, conversationID, msg string) (string, error) {
	f.mu.Lock()
	f.syncRuns++
	f.mu.Unlock()
	return "ok", f.err
}
func (f *fakeRunner) RunTurnPersisted(ctx context.Context, conversationID, msg string) (string, error) {
	f.mu.Lock()
	f.persistRuns++
	f.mu.Unlock()
	return "ok", f.err
}

// fakeMsgRepo simulates the provider-msg-id claim with an in-memory set so the
// ON CONFLICT semantics (first caller wins) are exercised without Postgres.
type fakeMsgRepo struct {
	mu      sync.Mutex
	claimed map[string]bool
}

func newFakeMsgRepo() *fakeMsgRepo { return &fakeMsgRepo{claimed: map[string]bool{}} }

func (r *fakeMsgRepo) Append(ctx context.Context, msg *domain.Message) error { return nil }
func (r *fakeMsgRepo) ListByConversation(ctx context.Context, conversationID string) ([]*domain.Message, error) {
	return nil, nil
}
func (r *fakeMsgRepo) SeenProviderMsg(ctx context.Context, providerMsgID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.claimed[providerMsgID], nil
}
func (r *fakeMsgRepo) ClaimProviderMsg(ctx context.Context, conversationID, providerMsgID, content string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimed[providerMsgID] {
		return false, nil
	}
	r.claimed[providerMsgID] = true
	return true, nil
}

// --- tests -------------------------------------------------------------------

// TestTurnWorker_IdempotentByProviderMsgID asserts the same provider message id
// runs the agent turn exactly once.
func TestTurnWorker_IdempotentByProviderMsgID(t *testing.T) {
	runner := &fakeRunner{}
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo())

	job := newProviderJob("conv-1", "wamid.1")
	for i := 0; i < 3; i++ {
		if err := w.Process(context.Background(), job); err != nil {
			t.Fatalf("Process #%d: %v", i, err)
		}
	}

	if runner.persistRuns != 1 {
		t.Fatalf("expected the turn to run once, ran %d times", runner.persistRuns)
	}
}

// TestTurnWorker_NoProviderIDAlwaysRuns asserts web turns (no provider id) are
// never deduped — today's behavior is preserved.
func TestTurnWorker_NoProviderIDAlwaysRuns(t *testing.T) {
	runner := &fakeRunner{}
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo())

	job := newJob("conv-1", "salut")
	for i := 0; i < 3; i++ {
		if err := w.Process(context.Background(), job); err != nil {
			t.Fatalf("Process #%d: %v", i, err)
		}
	}

	if runner.syncRuns != 3 {
		t.Fatalf("expected 3 sync runs (no dedupe), got %d", runner.syncRuns)
	}
	if runner.persistRuns != 0 {
		t.Fatalf("expected 0 persisted runs, got %d", runner.persistRuns)
	}
}

// TestTurnWorker_RetryableErrorPropagates asserts a plain turn error bubbles out
// (the transport should retry).
func TestTurnWorker_RetryableErrorPropagates(t *testing.T) {
	runner := &fakeRunner{err: errors.New("llm 503")}
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo())

	err := w.Process(context.Background(), newJob("conv-1", "x"))
	if err == nil {
		t.Fatal("expected retryable error to propagate, got nil")
	}
}

// TestTurnWorker_TerminalErrorSwallowed asserts a TerminalError-wrapped failure
// is logged and acked (not retried).
func TestTurnWorker_TerminalErrorSwallowed(t *testing.T) {
	runner := &fakeRunner{err: TerminalError(errors.New("unknown tool"))}
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo())

	if err := w.Process(context.Background(), newJob("conv-1", "x")); err != nil {
		t.Fatalf("expected terminal error to be swallowed, got %v", err)
	}
}
