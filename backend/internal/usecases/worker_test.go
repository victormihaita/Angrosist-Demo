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

// recordingReplier records the lifecycle calls a turn delivers through it.
type recordingReplier struct {
	mu       sync.Mutex
	typing   int
	delivers []ports.Event
	errors   []string
	convs    []string
}

func (r *recordingReplier) Typing(_ context.Context, conv *domain.Conversation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.typing++
	r.convs = append(r.convs, conv.Channel)
	return nil
}
func (r *recordingReplier) Deliver(_ context.Context, _ *domain.Conversation, ev ports.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delivers = append(r.delivers, ev)
	return nil
}
func (r *recordingReplier) Error(_ context.Context, _ *domain.Conversation, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, message)
	return nil
}

// stubRegistry routes every channel to the same recordingReplier and records the
// channel name it was asked for.
type stubRegistry struct {
	replier  *recordingReplier
	askedFor []string
}

func (s *stubRegistry) For(channel string) ports.Replier {
	s.askedFor = append(s.askedFor, channel)
	return s.replier
}

// stubConvRepo returns a fixed conversation for GetByID (the worker loads it to
// route the reply by channel).
type stubConvRepo struct {
	conv *domain.Conversation
}

func (r *stubConvRepo) Create(context.Context, string) (*domain.Conversation, error) {
	return r.conv, nil
}
func (r *stubConvRepo) CreateWith(context.Context, string, string, string) (*domain.Conversation, error) {
	return r.conv, nil
}
func (r *stubConvRepo) GetByID(context.Context, string) (*domain.Conversation, error) {
	return r.conv, nil
}
func (r *stubConvRepo) UpdateState(context.Context, string, domain.ConversationState) error {
	return nil
}
func (r *stubConvRepo) UpdateExtracted(context.Context, string, map[string]any) error { return nil }
func (r *stubConvRepo) SetBotActive(context.Context, string, bool) error              { return nil }
func (r *stubConvRepo) SetContact(context.Context, string, string) error              { return nil }
func (r *stubConvRepo) GetOrCreateByChannelPhone(context.Context, string, string, string, string) (*domain.Conversation, error) {
	return r.conv, nil
}
func (r *stubConvRepo) ContactPhoneByConversation(context.Context, string) (string, error) {
	return "", nil
}

// --- tests -------------------------------------------------------------------

// TestTurnWorker_RoutesReplyByChannel asserts the worker resolves the
// conversation's channel through the registry and delivers typing + the completed
// reply through that channel's Replier.
func TestTurnWorker_RoutesReplyByChannel(t *testing.T) {
	rep := &recordingReplier{}
	reg := &stubRegistry{replier: rep}
	conv := &domain.Conversation{ID: "conv-1", Channel: "whatsapp", State: domain.StateQualifying}
	w := NewTurnWorker(&fakeRunner{}, lock.NewMemory(), newFakeMsgRepo(), &stubConvRepo{conv: conv}, reg)

	if err := w.Process(context.Background(), newProviderJob("conv-1", "wamid.1")); err != nil {
		t.Fatalf("process: %v", err)
	}

	if rep.typing != 1 {
		t.Fatalf("expected 1 typing event, got %d", rep.typing)
	}
	if len(rep.delivers) != 1 || rep.delivers[0].Reply != "ok" {
		t.Fatalf("expected 1 deliver with reply 'ok', got %+v", rep.delivers)
	}
	for _, c := range reg.askedFor {
		if c != "whatsapp" {
			t.Fatalf("registry asked for channel %q, want whatsapp", c)
		}
	}
}

// TestTurnWorker_DeliversErrorThroughChannel asserts a retryable turn error is
// reported to the channel's Replier as a safe error event (and still propagates).
func TestTurnWorker_DeliversErrorThroughChannel(t *testing.T) {
	rep := &recordingReplier{}
	reg := &stubRegistry{replier: rep}
	conv := &domain.Conversation{ID: "conv-1", Channel: "web"}
	w := NewTurnWorker(&fakeRunner{err: errors.New("boom")}, lock.NewMemory(), newFakeMsgRepo(), &stubConvRepo{conv: conv}, reg)

	if err := w.Process(context.Background(), newJob("conv-1", "x")); err == nil {
		t.Fatal("expected retryable error to propagate")
	}
	if len(rep.errors) != 1 {
		t.Fatalf("expected 1 error delivery, got %d", len(rep.errors))
	}
}

// TestTurnWorker_IdempotentByProviderMsgID asserts the same provider message id
// runs the agent turn exactly once.
func TestTurnWorker_IdempotentByProviderMsgID(t *testing.T) {
	runner := &fakeRunner{}
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo(), nil, nil)

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
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo(), nil, nil)

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
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo(), nil, nil)

	err := w.Process(context.Background(), newJob("conv-1", "x"))
	if err == nil {
		t.Fatal("expected retryable error to propagate, got nil")
	}
}

// TestTurnWorker_TerminalErrorSwallowed asserts a TerminalError-wrapped failure
// is logged and acked (not retried).
func TestTurnWorker_TerminalErrorSwallowed(t *testing.T) {
	runner := &fakeRunner{err: TerminalError(errors.New("unknown tool"))}
	w := NewTurnWorker(runner, lock.NewMemory(), newFakeMsgRepo(), nil, nil)

	if err := w.Process(context.Background(), newJob("conv-1", "x")); err != nil {
		t.Fatalf("expected terminal error to be swallowed, got %v", err)
	}
}
