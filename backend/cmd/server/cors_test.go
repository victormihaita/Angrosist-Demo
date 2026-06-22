package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCorsPreflight verifies the global CORS wrapper answers a browser preflight
// (OPTIONS) with 204 + CORS headers BEFORE routing/auth, and that the preflight
// allows the Authorization header the dashboard sends. Regression test for the
// dashboard "Preflight 405/401" failure.
func TestCorsPreflight(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := corsPreflight(next)

	// Preflight for an authed dashboard GET.
	req := httptest.NewRequest(http.MethodOptions, "/api/leads?limit=25", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d; want 204", rec.Code)
	}
	if called {
		t.Fatal("preflight must not reach the wrapped handler (routing/auth)")
	}
	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	if !containsFold(allowHeaders, "Authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q; must include Authorization", allowHeaders)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("preflight missing Access-Control-Allow-Origin")
	}

	// A real request passes through with CORS headers set.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/leads", nil))
	if !called {
		t.Fatal("non-OPTIONS request must reach the wrapped handler")
	}
	if rec2.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("response missing Access-Control-Allow-Origin")
	}
}

func containsFold(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if eqFold(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
