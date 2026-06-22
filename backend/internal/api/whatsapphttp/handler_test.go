package whatsapphttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

const testAppSecret = "app-secret"
const testVerifyToken = "verify-tok"

// fakeResolver creates a deterministic conversation per phone and records calls.
type fakeResolver struct {
	mu      sync.Mutex
	byPhone map[string]*domain.Conversation
	calls   int
	nextID  int
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{byPhone: map[string]*domain.Conversation{}}
}

func (r *fakeResolver) GetOrCreateByChannelPhone(_ context.Context, channel, phone, vertical, intent string) (*domain.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if c, ok := r.byPhone[phone]; ok {
		return c, nil
	}
	r.nextID++
	c := &domain.Conversation{ID: "conv-" + phone, Channel: channel, Vertical: vertical, Intent: intent}
	r.byPhone[phone] = c
	return c, nil
}

// fakeQueue records enqueued jobs.
type fakeQueue struct {
	mu   sync.Mutex
	jobs []ports.TurnJob
}

func (q *fakeQueue) Enqueue(_ context.Context, job ports.TurnJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, job)
	return nil
}

func sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(testAppSecret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newHandler() (*Handler, *fakeResolver, *fakeQueue) {
	res := newFakeResolver()
	q := &fakeQueue{}
	h := NewHandler(Config{AppSecret: testAppSecret, VerifyToken: testVerifyToken}, res, q)
	return h, res, q
}

// --- GET verify handshake ----------------------------------------------------

func TestVerify_CorrectTokenEchoesChallenge(t *testing.T) {
	h, _, _ := newHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token="+testVerifyToken+"&hub.challenge=12345", nil)
	rec := httptest.NewRecorder()

	h.Verify(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "12345" {
		t.Fatalf("expected challenge echoed verbatim, got %q", rec.Body.String())
	}
}

func TestVerify_WrongTokenForbidden(t *testing.T) {
	h, _, _ := newHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=12345", nil)
	rec := httptest.NewRecorder()

	h.Verify(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// --- POST inbound ------------------------------------------------------------

const inboundBody = `{"entry":[{"changes":[{"value":{"messages":[
	{"from":"40712345678","id":"wamid.ABC","type":"text","text":{"body":"salut"}}
]}}]}]}`

func postInbound(t *testing.T, h *Handler, body string, signature string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/whatsapp", strings.NewReader(body))
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rec := httptest.NewRecorder()
	h.Receive(rec, req)
	return rec
}

func TestReceive_ValidSignatureResolvesAndEnqueues(t *testing.T) {
	h, res, q := newHandler()
	rec := postInbound(t, h, inboundBody, sign([]byte(inboundBody)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected fast-ack 200, got %d", rec.Code)
	}
	if res.calls != 1 {
		t.Fatalf("expected 1 conversation resolution, got %d", res.calls)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("expected 1 enqueued turn, got %d", len(q.jobs))
	}
	job := q.jobs[0]
	if job.ConversationID != "conv-40712345678" {
		t.Errorf("conversation id = %q", job.ConversationID)
	}
	if job.ProviderMsgID != "wamid.ABC" {
		t.Errorf("provider_msg_id = %q, want wamid.ABC", job.ProviderMsgID)
	}
	if job.Message != "salut" {
		t.Errorf("message = %q", job.Message)
	}
}

func TestReceive_InvalidSignatureForbidden(t *testing.T) {
	h, res, q := newHandler()
	rec := postInbound(t, h, inboundBody, "sha256=deadbeef")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on bad signature, got %d", rec.Code)
	}
	if res.calls != 0 || len(q.jobs) != 0 {
		t.Fatal("must not resolve or enqueue on signature mismatch")
	}
}

func TestReceive_MissingSignatureForbidden(t *testing.T) {
	h, _, q := newHandler()
	rec := postInbound(t, h, inboundBody, "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on missing signature, got %d", rec.Code)
	}
	if len(q.jobs) != 0 {
		t.Fatal("must not enqueue without a signature")
	}
}

func TestReceive_StatusCallbackAcksWithoutEnqueue(t *testing.T) {
	statusBody := `{"entry":[{"changes":[{"value":{"statuses":[{"id":"wamid.ABC","status":"delivered"}]}}]}]}`
	h, res, q := newHandler()
	rec := postInbound(t, h, statusBody, sign([]byte(statusBody)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 ack for status callback, got %d", rec.Code)
	}
	if res.calls != 0 || len(q.jobs) != 0 {
		t.Fatal("status callback must not resolve or enqueue")
	}
}

func TestReceive_SamePhoneReusesConversation(t *testing.T) {
	h, _, q := newHandler()

	body1 := `{"entry":[{"changes":[{"value":{"messages":[{"from":"4071","id":"wamid.1","type":"text","text":{"body":"unu"}}]}}]}]}`
	body2 := `{"entry":[{"changes":[{"value":{"messages":[{"from":"4071","id":"wamid.2","type":"text","text":{"body":"doi"}}]}}]}]}`

	if rec := postInbound(t, h, body1, sign([]byte(body1))); rec.Code != http.StatusOK {
		t.Fatalf("first: %d", rec.Code)
	}
	if rec := postInbound(t, h, body2, sign([]byte(body2))); rec.Code != http.StatusOK {
		t.Fatalf("second: %d", rec.Code)
	}

	if len(q.jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(q.jobs))
	}
	if q.jobs[0].ConversationID != q.jobs[1].ConversationID {
		t.Fatal("same phone should resolve to the same conversation")
	}
	if q.jobs[0].ProviderMsgID == q.jobs[1].ProviderMsgID {
		t.Fatal("the two messages have distinct provider ids")
	}
}
