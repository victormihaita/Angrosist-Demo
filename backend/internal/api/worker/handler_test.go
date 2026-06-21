package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angrosist/demo/internal/ports"
)

type fakeProcessor struct {
	jobs []ports.TurnJob
	err  error
}

func (f *fakeProcessor) Process(ctx context.Context, job ports.TurnJob) error {
	f.jobs = append(f.jobs, job)
	return f.err
}

// TestHandler_DecodesJobAndInvokesProcessor asserts a well-formed POST is decoded
// into a TurnJob, passed to the processor, and acked with 200.
func TestHandler_DecodesJobAndInvokesProcessor(t *testing.T) {
	fp := &fakeProcessor{}
	h := NewHandler(fp)

	body := `{"conversation_id":"conv-1","message":"salut","provider_msg_id":"wamid.1"}`
	req := httptest.NewRequest(http.MethodPost, "/worker/turn", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(fp.jobs) != 1 {
		t.Fatalf("expected 1 job processed, got %d", len(fp.jobs))
	}
	got := fp.jobs[0]
	if got.ConversationID != "conv-1" || got.Message != "salut" || got.ProviderMsgID != "wamid.1" {
		t.Fatalf("decoded job mismatch: %+v", got)
	}
}

// TestHandler_RetryableReturns500 asserts a retryable processor error maps to 500
// so the transport redelivers.
func TestHandler_RetryableReturns500(t *testing.T) {
	fp := &fakeProcessor{err: errors.New("llm 503")}
	h := NewHandler(fp)

	req := httptest.NewRequest(http.MethodPost, "/worker/turn",
		strings.NewReader(`{"conversation_id":"conv-1","message":"x"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for retryable error, got %d", rec.Code)
	}
}

// TestHandler_MalformedReturns400 asserts a bad body is rejected terminally.
func TestHandler_MalformedReturns400(t *testing.T) {
	h := NewHandler(&fakeProcessor{})
	req := httptest.NewRequest(http.MethodPost, "/worker/turn", strings.NewReader(`{not json`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed body, got %d", rec.Code)
	}
}

// TestHandler_MissingConversationIDReturns400 asserts an empty conversation id is
// rejected terminally (it cannot be locked on).
func TestHandler_MissingConversationIDReturns400(t *testing.T) {
	h := NewHandler(&fakeProcessor{})
	req := httptest.NewRequest(http.MethodPost, "/worker/turn",
		strings.NewReader(`{"message":"x"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing conversation_id, got %d", rec.Code)
	}
}
