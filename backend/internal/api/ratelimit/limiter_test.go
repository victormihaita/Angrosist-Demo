package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeClock lets tests advance time deterministically.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) add(d time.Duration) {
	c.t = c.t.Add(d)
}

// newTestLimiter builds a limiter with an injected clock.
func newTestLimiter(rpm, burst int, clk *fakeClock) *Limiter {
	l := New(rpm, burst)
	l.now = clk.now
	return l
}

func TestLimiter_NPassThenReject(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	// 60 rpm, burst 5: the first 5 requests in the same instant pass, the 6th fails.
	l := newTestLimiter(60, 5, clk)

	for i := 0; i < 5; i++ {
		ok, _ := l.Allow("1.2.3.4")
		if !ok {
			t.Fatalf("request %d should pass within burst", i+1)
		}
	}
	ok, retry := l.Allow("1.2.3.4")
	if ok {
		t.Fatal("6th request should be rejected (burst exhausted)")
	}
	if retry <= 0 {
		t.Fatalf("rejected request should report a positive Retry-After, got %v", retry)
	}
}

func TestLimiter_WindowResets(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	// 60 rpm = 1 token/sec, burst 1.
	l := newTestLimiter(60, 1, clk)

	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("first request should pass")
	}
	if ok, _ := l.Allow("ip"); ok {
		t.Fatal("second immediate request should be rejected")
	}
	// Advance one second -> one token refilled.
	clk.add(time.Second)
	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("request after refill window should pass")
	}
}

func TestLimiter_IndependentKeys(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newTestLimiter(60, 1, clk)

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("ip a first request should pass")
	}
	// ip a is now exhausted, but ip b has its own bucket.
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("ip a second request should be rejected")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("ip b should pass independently of ip a")
	}
}

func TestLimiter_Disabled(t *testing.T) {
	l := New(0, 0) // RPM 0 => disabled
	for i := 0; i < 1000; i++ {
		if ok, _ := l.Allow("ip"); !ok {
			t.Fatal("disabled limiter must always allow")
		}
	}
}

func TestLimiter_Cleanup(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newTestLimiter(60, 1, clk)
	l.ttl = time.Minute

	l.Allow("stale")
	if len(l.buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(l.buckets))
	}
	clk.add(2 * time.Minute)
	l.cleanup()
	if len(l.buckets) != 0 {
		t.Fatalf("stale bucket should have been reclaimed, got %d", len(l.buckets))
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newTestLimiter(60, 1, clk) // burst 1: 2nd immediate request is 429

	var calls int
	next := func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}
	h := RateLimit(l, next)

	doReq := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/chat", nil)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}

	if rec := doReq("9.9.9.9"); rec.Code != http.StatusOK {
		t.Fatalf("first request status = %d; want 200", rec.Code)
	}
	rec := doReq("9.9.9.9")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d; want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 response must carry a Retry-After header")
	}
	if calls != 1 {
		t.Fatalf("next handler called %d times; want 1 (the throttled request must not reach it)", calls)
	}

	// A different IP is independent and passes.
	if rec := doReq("8.8.8.8"); rec.Code != http.StatusOK {
		t.Fatalf("different IP status = %d; want 200", rec.Code)
	}
}

func TestRateLimitMiddleware_SkipsOptions(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newTestLimiter(60, 1, clk)
	var calls int
	h := RateLimit(l, func(w http.ResponseWriter, r *http.Request) { calls++ })

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodOptions, "/api/chat", nil)
		req.RemoteAddr = "7.7.7.7:1"
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatal("OPTIONS preflight must never be rate-limited")
		}
	}
	if calls != 5 {
		t.Fatalf("OPTIONS requests reached next %d times; want 5", calls)
	}
}
