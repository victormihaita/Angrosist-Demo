package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angrosist/demo/internal/ports"
)

// stubProcessor records whether Process ran so tests can assert the auth guard
// short-circuits before the processor on rejection.
type stubProcessor struct{ ran bool }

func (p *stubProcessor) Process(ctx context.Context, job ports.TurnJob) error {
	p.ran = true
	return nil
}

const validJobBody = `{"conversation_id":"11111111-1111-1111-1111-111111111111","message":"hi"}`

func postTurn(t *testing.T, h http.HandlerFunc, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/worker/turn", strings.NewReader(validJobBody))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestWorkerAuth_TokenSet(t *testing.T) {
	const token = "s3cr3t-worker-token"

	t.Run("correct bearer passes", func(t *testing.T) {
		proc := &stubProcessor{}
		h := NewAuthenticatedHandler(proc, token)
		if !h.AuthConfigured() {
			t.Fatal("AuthConfigured() = false; want true when token is set")
		}
		rec := postTurn(t, h.TurnRoute(), "Bearer "+token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200 (body=%s)", rec.Code, rec.Body.String())
		}
		if !proc.ran {
			t.Fatal("processor did not run for an authenticated request")
		}
	})

	t.Run("case-insensitive scheme passes", func(t *testing.T) {
		proc := &stubProcessor{}
		h := NewAuthenticatedHandler(proc, token)
		rec := postTurn(t, h.TurnRoute(), "bearer "+token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200", rec.Code)
		}
		if !proc.ran {
			t.Fatal("processor did not run")
		}
	})

	t.Run("wrong token is 401 and processor not run", func(t *testing.T) {
		proc := &stubProcessor{}
		h := NewAuthenticatedHandler(proc, token)
		rec := postTurn(t, h.TurnRoute(), "Bearer wrong-token")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rec.Code)
		}
		if proc.ran {
			t.Fatal("processor ran for a rejected request")
		}
	})

	t.Run("missing header is 401", func(t *testing.T) {
		proc := &stubProcessor{}
		h := NewAuthenticatedHandler(proc, token)
		rec := postTurn(t, h.TurnRoute(), "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rec.Code)
		}
		if proc.ran {
			t.Fatal("processor ran for a request with no Authorization header")
		}
	})

	t.Run("non-bearer scheme is 401", func(t *testing.T) {
		proc := &stubProcessor{}
		h := NewAuthenticatedHandler(proc, token)
		rec := postTurn(t, h.TurnRoute(), "Basic "+token)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rec.Code)
		}
	})
}

func TestWorkerAuth_TokenUnset(t *testing.T) {
	proc := &stubProcessor{}
	h := NewAuthenticatedHandler(proc, "")
	if h.AuthConfigured() {
		t.Fatal("AuthConfigured() = true; want false when token is empty")
	}
	// No Authorization header — must still be allowed in the unauthenticated
	// (local/dev) mode. cmd/worker logs the startup warning in this case.
	rec := postTurn(t, h.TurnRoute(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 when auth is disabled (body=%s)", rec.Code, rec.Body.String())
	}
	if !proc.ran {
		t.Fatal("processor did not run with auth disabled")
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":  "abc",
		"bearer abc":  "abc",
		"BEARER abc":  "abc",
		"Bearer  abc": "abc",
		"":            "",
		"abc":         "",
		"Basic abc":   "",
		"Bearer":      "",
	}
	for header, want := range cases {
		if got := bearerToken(header); got != want {
			t.Errorf("bearerToken(%q) = %q; want %q", header, got, want)
		}
	}
}
