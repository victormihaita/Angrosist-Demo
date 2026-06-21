package ports

import "context"

// Locker serializes work per conversation so two agent turns for the same
// conversation never run concurrently (preserving message ordering and avoiding
// double-writes), while turns for different conversations run in parallel.
//
// The production adapter uses a PostgreSQL advisory lock; an in-memory adapter is
// used in tests. The agent core and use-cases depend only on this interface.
type Locker interface {
	// WithConversationLock acquires the lock for conversationID, invokes fn while
	// holding it, and releases it before returning. fn's error is propagated
	// unchanged so callers keep the retryable/terminal distinction. An error
	// acquiring or releasing the lock is wrapped with context and returned.
	WithConversationLock(ctx context.Context, conversationID string, fn func(context.Context) error) error
}
