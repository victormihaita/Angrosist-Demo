// Package ratelimit provides a small, dependency-free per-client-IP rate limiter
// and an HTTP middleware that applies it to the public, expensive endpoints
// (POST /api/chat, POST /api/conversations/{id}/photos). It bounds LLM
// cost-exhaustion and abuse from a single client (SECURITY.md §1.1 D).
//
// Design: a token bucket per client IP, guarded by a single mutex, with periodic
// cleanup of stale buckets so memory does not grow unbounded. No external
// dependency — stdlib only.
//
// Scope: this limiter is PER-INSTANCE (one Cloud Run instance / local process).
// It is the portable, always-on baseline. A multi-instance production deployment
// should additionally rely on Cloudflare WAF rate limiting at the edge, or swap a
// Redis-backed limiter behind the SAME middleware seam (RateLimit) — the HTTP
// boundary does not change.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a thread-safe token-bucket rate limiter keyed by an arbitrary string
// (the client IP at the HTTP edge). Each key refills at rate tokens per minute up
// to a burst capacity. A zero-value Limiter must not be used; construct with New.
type Limiter struct {
	// ratePerMin is the sustained allowance (tokens added per minute).
	ratePerMin int
	// burst is the bucket capacity (max tokens, i.e. the instantaneous burst).
	burst int
	// refillPerNano is tokens added per nanosecond, precomputed from ratePerMin.
	refillPerNano float64

	mu      sync.Mutex
	buckets map[string]*bucket

	// now is injectable for deterministic tests; defaults to time.Now.
	now func() time.Time
	// ttl is how long an idle bucket is kept before cleanup reclaims it.
	ttl time.Duration
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New constructs a Limiter allowing ratePerMin requests per minute per key with
// the given burst capacity. A non-positive burst defaults to ratePerMin (or 1 if
// that is also non-positive) so a single request is always admitted. A
// non-positive ratePerMin yields a disabled limiter (Allow always returns true);
// callers should prefer not constructing one at all in that case.
func New(ratePerMin, burst int) *Limiter {
	if burst <= 0 {
		burst = ratePerMin
	}
	if burst <= 0 {
		burst = 1
	}
	l := &Limiter{
		ratePerMin: ratePerMin,
		burst:      burst,
		buckets:    make(map[string]*bucket),
		now:        time.Now,
		ttl:        10 * time.Minute,
	}
	if ratePerMin > 0 {
		l.refillPerNano = float64(ratePerMin) / float64(time.Minute)
	}
	return l
}

// Allow reports whether a request for key may proceed, consuming one token when
// it does. When the limiter is disabled (ratePerMin <= 0) it always allows. The
// second return is a hint for the Retry-After header: the duration until at least
// one token will be available (zero when allowed).
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l.ratePerMin <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		// First request from this key: full bucket, consume one.
		l.buckets[key] = &bucket{tokens: float64(l.burst) - 1, last: now}
		return true, 0
	}

	// Refill based on elapsed time since the last observation.
	elapsed := now.Sub(b.last)
	if elapsed > 0 {
		b.tokens += float64(elapsed) * l.refillPerNano
		if b.tokens > float64(l.burst) {
			b.tokens = float64(l.burst)
		}
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Time until the next whole token is available.
	needed := 1 - b.tokens
	wait := time.Duration(needed / l.refillPerNano)
	if wait < time.Second {
		wait = time.Second
	}
	return false, wait
}

// cleanup removes buckets idle longer than ttl. It is invoked periodically by the
// goroutine started in StartCleanup; it is also safe to call directly in tests.
func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-l.ttl)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// StartCleanup launches a background goroutine that periodically reclaims stale
// buckets until stop is closed. It is started once from the composition root. The
// limiter remains usable without it (memory simply grows with distinct IPs).
func (l *Limiter) StartCleanup(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = l.ttl
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.cleanup()
			case <-stop:
				return
			}
		}
	}()
}
