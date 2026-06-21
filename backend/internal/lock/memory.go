// Package lock provides an in-process implementation of the ports.Locker port.
// It is used in tests and as a fallback when no external lock backend is wired.
package lock

import (
	"context"
	"sync"

	"github.com/angrosist/demo/internal/ports"
)

var _ ports.Locker = (*Memory)(nil)

// Memory is an in-process ports.Locker backed by a per-conversation mutex map.
// It serializes turns for the same conversation id within a single process and
// allows different conversation ids to run concurrently. It is not suitable for
// multi-instance deployments (use the Postgres advisory-lock adapter there) but
// is correct and dependency-free for tests and the single-instance local demo.
type Memory struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewMemory constructs an in-process Locker.
func NewMemory() *Memory {
	return &Memory{locks: make(map[string]*sync.Mutex)}
}

func (m *Memory) lockFor(conversationID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[conversationID]
	if !ok {
		l = &sync.Mutex{}
		m.locks[conversationID] = l
	}
	return l
}

// WithConversationLock acquires the per-conversation mutex, runs fn, and releases
// it. fn's error is returned unchanged so callers keep the retryable/terminal
// distinction.
func (m *Memory) WithConversationLock(ctx context.Context, conversationID string, fn func(context.Context) error) error {
	l := m.lockFor(conversationID)
	l.Lock()
	defer l.Unlock()
	return fn(ctx)
}
