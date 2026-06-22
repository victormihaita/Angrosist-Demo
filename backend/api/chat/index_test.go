package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angrosist/demo/internal/app"
)

// bootContainer initializes the singleton container with the minimal env the chat
// handler's ownership check needs. JWT_SECRET is required (auth fails fast without
// it); CONVERSATION_TOKEN_SECRET pins a deterministic key for the test. DATABASE_URL
// is set to a dummy DSN — pgxpool.New is lazy, so no DB connection is made unless a
// turn actually runs, and the 403 path returns before any DB/LLM call.
//
// app.Init is a sync.Once, so these env vars must be set before the FIRST call to
// GetContainer in the test binary. This package's only test boots it here.
func bootContainer(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "chat-test-jwt-secret")
	t.Setenv("CONVERSATION_TOKEN_SECRET", "chat-test-conv-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:1/none?sslmode=disable")
	app.Init()
}

func postChat(t *testing.T, body map[string]string, token string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Conversation-Token", token)
	}
	rec := httptest.NewRecorder()
	Handler(rec, req)
	return rec
}

// TestChat_ContinuingTurn_MissingToken asserts a turn that carries a conversation_id
// but NO ownership token is refused 403 before any turn work (SECURITY.md §1.1).
func TestChat_ContinuingTurn_MissingToken(t *testing.T) {
	bootContainer(t)
	rec := postChat(t, map[string]string{
		"conversation_id": "11111111-1111-1111-1111-111111111111",
		"message":         "continua",
	}, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403 for missing token; body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Error.Code != "FORBIDDEN" {
		t.Fatalf("error code = %q; want FORBIDDEN", env.Error.Code)
	}
}

// TestChat_ContinuingTurn_WrongToken asserts a token bound to a DIFFERENT
// conversation is refused 403.
func TestChat_ContinuingTurn_WrongToken(t *testing.T) {
	bootContainer(t)
	const convID = "11111111-1111-1111-1111-111111111111"
	wrong := app.GetContainer().ConvToken.Issue("22222222-2222-2222-2222-222222222222")
	rec := postChat(t, map[string]string{
		"conversation_id": convID,
		"message":         "continua",
	}, wrong)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403 for wrong token; body=%s", rec.Code, rec.Body.String())
	}
}

// TestChat_FirstTurn_NoTokenRequired asserts a first turn (no conversation_id) is
// NOT rejected by the ownership check. It does not assert a 200 (that needs a DB);
// it asserts the request is not refused 403 — i.e. it proceeds past the token gate.
func TestChat_FirstTurn_NoTokenRequired(t *testing.T) {
	bootContainer(t)
	rec := postChat(t, map[string]string{"message": "vreau cafea"}, "")
	if rec.Code == http.StatusForbidden {
		t.Fatalf("first turn was refused 403 by the ownership check; body=%s", rec.Body.String())
	}
}
