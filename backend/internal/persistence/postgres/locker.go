package postgres

import (
	"context"
	"fmt"

	"github.com/angrosist/demo/internal/ports"
)

var _ ports.Locker = (*Locker)(nil)

// Locker is the PostgreSQL advisory-lock implementation of ports.Locker. It
// serializes agent turns per conversation so two turns for the same conversation
// never run concurrently, while different conversations proceed in parallel.
type Locker struct{}

// NewLocker constructs the Postgres advisory-lock adapter.
func NewLocker() *Locker { return &Locker{} }

// WithConversationLock acquires a session-level advisory lock keyed by the
// conversation id, runs fn while holding it, then releases the lock.
//
// Design choice — session-level lock on a dedicated connection (not xact-level):
//
//   - pg_advisory_xact_lock auto-releases at transaction end, but it would force
//     the entire agent turn to run inside one long-lived transaction on one
//     connection. The agent turn issues many independent queries (history, LLM
//     tool side effects, lead creation) through the shared pool; pinning all of
//     that to a single open transaction for the whole LLM round-trip is
//     undesirable (long-held transaction, no per-query autonomy).
//
//   - Instead we take a session-level lock (pg_advisory_lock) on a connection
//     acquired solely for locking, run fn (which uses the shared pool for its own
//     work), and release with pg_advisory_unlock via a deferred call. Session
//     locks are tied to the connection: if the worker crashes or the connection
//     drops, PostgreSQL releases the lock automatically, so no orphaned locks
//     survive — giving us crash-safety without holding a transaction open.
//
// hashtext maps the conversation id (a UUID string) to the bigint the advisory
// lock API requires.
func (l *Locker) WithConversationLock(ctx context.Context, conversationID string, fn func(context.Context) error) error {
	conn, err := GetPool().Acquire(ctx)
	if err != nil {
		return fmt.Errorf("locker: acquire conn for conversation %s: %w", conversationID, err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, conversationID); err != nil {
		return fmt.Errorf("locker: acquire advisory lock for conversation %s: %w", conversationID, err)
	}
	defer func() {
		// Release on a background-derived context so an already-cancelled ctx does
		// not prevent unlock; if it does fail, the session lock still dies with the
		// connection when it is released back to the pool's lifecycle.
		unlockCtx := context.WithoutCancel(ctx)
		if _, uerr := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, conversationID); uerr != nil {
			// Best-effort: the session lock is released when the connection closes.
			_ = uerr
		}
	}()

	return fn(ctx)
}
