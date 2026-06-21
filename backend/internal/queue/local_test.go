package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/angrosist/demo/internal/ports"
)

// fakeProcessor records the jobs it was asked to process and signals each one.
type fakeProcessor struct {
	mu   sync.Mutex
	jobs []ports.TurnJob
	done chan struct{}
}

func newFakeProcessor() *fakeProcessor {
	return &fakeProcessor{done: make(chan struct{}, 16)}
}

func (f *fakeProcessor) Process(ctx context.Context, job ports.TurnJob) error {
	f.mu.Lock()
	f.jobs = append(f.jobs, job)
	f.mu.Unlock()
	f.done <- struct{}{}
	return nil
}

// TestLocal_EnqueueRunsProcessor asserts the local queue dispatches the job to
// the registered processor.
func TestLocal_EnqueueRunsProcessor(t *testing.T) {
	fp := newFakeProcessor()
	q := NewLocal(fp)

	job := ports.TurnJob{ConversationID: "conv-1", Message: "salut", ProviderMsgID: "wamid.1"}
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	select {
	case <-fp.done:
	case <-time.After(2 * time.Second):
		t.Fatal("processor was not invoked within timeout")
	}

	fp.mu.Lock()
	defer fp.mu.Unlock()
	if len(fp.jobs) != 1 {
		t.Fatalf("expected 1 processed job, got %d", len(fp.jobs))
	}
	if fp.jobs[0] != job {
		t.Fatalf("processed job mismatch: got %+v want %+v", fp.jobs[0], job)
	}
}
