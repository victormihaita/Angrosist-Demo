package postgres

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// requireDB skips the test unless DATABASE_URL points at a reachable Postgres.
// The CI/local recipe spins up a scratch cluster (see the task notes) and exports
// DATABASE_URL before running these tests.
func requireDB(t *testing.T) {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := GetPool().Ping(ctx); err != nil {
		t.Skipf("Postgres not reachable: %v", err)
	}
}

// newConversation inserts a conversation row and returns its id so lock/claim
// tests operate on real foreign-key-valid data.
func newConversation(t *testing.T) string {
	t.Helper()
	var id string
	err := GetPool().QueryRow(context.Background(), `
		INSERT INTO conversations (channel, state, extracted)
		VALUES ('test', 'qualifying', '{}')
		RETURNING id
	`).Scan(&id)
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	return id
}

// TestLocker_SerializesSameConversation asserts the Postgres advisory lock
// prevents two critical sections for the same conversation id from overlapping.
func TestLocker_SerializesSameConversation(t *testing.T) {
	requireDB(t)
	l := NewLocker()
	conv := newConversation(t)

	var inside, maxInside, ran int32
	var wg sync.WaitGroup
	const goroutines = 6

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := l.WithConversationLock(context.Background(), conv, func(ctx context.Context) error {
				cur := atomic.AddInt32(&inside, 1)
				for {
					m := atomic.LoadInt32(&maxInside)
					if cur <= m || atomic.CompareAndSwapInt32(&maxInside, m, cur) {
						break
					}
				}
				time.Sleep(15 * time.Millisecond)
				atomic.AddInt32(&ran, 1)
				atomic.AddInt32(&inside, -1)
				return nil
			})
			if err != nil {
				t.Errorf("WithConversationLock: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxInside); got != 1 {
		t.Fatalf("advisory lock allowed concurrency for same conversation: peak=%d, want 1", got)
	}
	if got := atomic.LoadInt32(&ran); got != goroutines {
		t.Fatalf("expected %d sections to run, got %d", goroutines, got)
	}
}

// TestLocker_DifferentConversationsConcurrent asserts distinct conversation ids
// are not serialized against each other (no global bottleneck).
func TestLocker_DifferentConversationsConcurrent(t *testing.T) {
	requireDB(t)
	l := NewLocker()

	const n = 4
	ids := make([]string, n)
	for i := range ids {
		ids[i] = newConversation(t)
	}

	reached := make(chan struct{}, n)
	release := make(chan struct{})
	var wg sync.WaitGroup

	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.WithConversationLock(context.Background(), id, func(ctx context.Context) error {
				reached <- struct{}{}
				<-release
				return nil
			})
		}()
	}

	timeout := time.After(3 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-reached:
		case <-timeout:
			t.Fatalf("different conversations did not run concurrently: %d/%d entered", i, n)
		}
	}
	close(release)
	wg.Wait()
}

// TestClaimProviderMsg_Idempotent asserts the partial-unique index makes the
// claim win exactly once for a given provider message id, even concurrently.
func TestClaimProviderMsg_Idempotent(t *testing.T) {
	requireDB(t)
	repo := NewMessageRepo()
	conv := newConversation(t)
	providerID := "wamid.test." + conv // unique per run via the fresh conversation id

	const goroutines = 10
	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := repo.ClaimProviderMsg(context.Background(), conv, providerID, "hello")
			if err != nil {
				t.Errorf("ClaimProviderMsg: %v", err)
				return
			}
			if claimed {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("expected exactly one claim to win, got %d", wins)
	}

	// And a later check reports it as seen.
	seen, err := repo.SeenProviderMsg(context.Background(), providerID)
	if err != nil {
		t.Fatalf("SeenProviderMsg: %v", err)
	}
	if !seen {
		t.Fatal("expected provider message to be seen after claim")
	}

	// Exactly one user message row was persisted for this provider id.
	var count int
	if err := GetPool().QueryRow(context.Background(),
		`SELECT count(*) FROM messages WHERE provider_msg_id = $1`, providerID).Scan(&count); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 persisted message for provider id, got %d", count)
	}
}
