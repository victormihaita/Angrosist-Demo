package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/angrosist/demo/internal/broker"
	"github.com/angrosist/demo/internal/convtoken"
	"github.com/angrosist/demo/internal/ports"
)

// streamIssuer is the shared conversation-token issuer for the stream tests.
var streamIssuer = convtoken.New("stream-test-secret", "")

// TestSSEStreamsMessageEvent runs the handler in a goroutine, publishes a message
// event to the broker, and asserts it is written to the response stream in the
// correct event:/data: SSE framing. The request context is cancelled to stop the
// handler cleanly (simulating client disconnect).
func TestSSEStreamsMessageEvent(t *testing.T) {
	b := broker.NewInProcess()
	h := Handler(b, streamIssuer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := "/api/stream?conversation_id=conv-1&token=" + streamIssuer.Issue("conv-1")
	req := httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h(rec, req)
		close(done)
	}()

	// Wait until the handler has subscribed before publishing, so the event is not
	// dropped. Poll the broker by retrying the publish until the recorder shows it.
	deadline := time.After(2 * time.Second)
	published := false
	for !published {
		select {
		case <-deadline:
			t.Fatal("timed out before the event was streamed")
		default:
		}
		b.Publish("conv-1", ports.Event{
			Type:      ports.EventMessage,
			Reply:     "hello",
			State:     "qualifying",
			Extracted: map[string]any{"product_name": "widgets"},
		})
		time.Sleep(10 * time.Millisecond)
		if strings.Contains(rec.Body.String(), "event: message") {
			published = true
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: message\n") {
		t.Fatalf("missing event line in SSE output:\n%s", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Fatalf("missing data line in SSE output:\n%s", body)
	}
	// The data payload is the JSON-encoded event terminated by a blank line.
	if !strings.Contains(body, `"reply":"hello"`) {
		t.Fatalf("data payload missing reply field:\n%s", body)
	}
	if !strings.Contains(body, "\n\n") {
		t.Fatalf("SSE frame not terminated by blank line:\n%s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got %q", ct)
	}
}

// TestSSEMissingConversationID asserts the handler rejects a request without a
// conversation_id.
func TestSSEMissingConversationID(t *testing.T) {
	h := Handler(broker.NewInProcess(), streamIssuer)
	req := httptest.NewRequest(http.MethodGet, "/api/stream", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing conversation_id, got %d", rec.Code)
	}
}

// TestSSEMissingToken asserts a subscribe with a known conversation_id but NO
// token is refused 403 (SECURITY.md §1.1 — no cross-conversation reads).
func TestSSEMissingToken(t *testing.T) {
	h := Handler(broker.NewInProcess(), streamIssuer)
	req := httptest.NewRequest(http.MethodGet, "/api/stream?conversation_id=conv-1", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing token, got %d", rec.Code)
	}
}

// TestSSEWrongToken asserts a subscribe with a token bound to a DIFFERENT
// conversation is refused 403.
func TestSSEWrongToken(t *testing.T) {
	h := Handler(broker.NewInProcess(), streamIssuer)
	url := "/api/stream?conversation_id=conv-1&token=" + streamIssuer.Issue("conv-2")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong token, got %d", rec.Code)
	}
}
