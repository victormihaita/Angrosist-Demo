package lock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMemory_SerializesSameConversation asserts two critical sections for the
// same conversation id never overlap.
func TestMemory_SerializesSameConversation(t *testing.T) {
	l := NewMemory()

	var inside int32    // currently inside the critical section
	var maxInside int32 // peak concurrency observed
	var ran int32       // total sections run
	var wg sync.WaitGroup

	const convID = "conv-A"
	const goroutines = 8

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.WithConversationLock(context.Background(), convID, func(ctx context.Context) error {
				cur := atomic.AddInt32(&inside, 1)
				for {
					m := atomic.LoadInt32(&maxInside)
					if cur <= m || atomic.CompareAndSwapInt32(&maxInside, m, cur) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond) // widen the window for overlap to show
				atomic.AddInt32(&ran, 1)
				atomic.AddInt32(&inside, -1)
				return nil
			})
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxInside); got != 1 {
		t.Fatalf("same-conversation lock allowed concurrency: peak inside=%d, want 1", got)
	}
	if got := atomic.LoadInt32(&ran); got != goroutines {
		t.Fatalf("expected all %d sections to run, got %d", goroutines, got)
	}
}

// TestMemory_DifferentConversationsRunConcurrently asserts distinct conversation
// ids are not serialized against each other.
func TestMemory_DifferentConversationsRunConcurrently(t *testing.T) {
	l := NewMemory()

	const n = 6
	start := make(chan struct{})
	reached := make(chan struct{}, n)
	release := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		id := string(rune('A' + i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = l.WithConversationLock(context.Background(), id, func(ctx context.Context) error {
				reached <- struct{}{}
				<-release // hold the lock until told to release
				return nil
			})
		}()
	}

	close(start)

	// All n distinct-conversation sections must be inside concurrently.
	timeout := time.After(2 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-reached:
		case <-timeout:
			t.Fatalf("different conversations did not run concurrently: only %d of %d entered", i, n)
		}
	}
	close(release)
	wg.Wait()
}
