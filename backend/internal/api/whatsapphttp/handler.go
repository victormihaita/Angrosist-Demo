// Package whatsapphttp is the inbound WhatsApp webhook HTTP surface: the Meta
// subscription verification handshake (GET) and the signed inbound message
// webhook (POST). It follows the async ack-fast contract (ARCHITECTURE_DETAIL
// §4): verify the signature first, parse, resolve/create the conversation, dedupe
// + enqueue a turn, then return 200 fast — never running the LLM inline.
//
// The route is PUBLIC (no bearer auth) but every POST is authenticated by the
// X-Hub-Signature-256 HMAC over the raw body; a missing/invalid signature is a
// 403 (and a logged security event). No PII is logged — only message ids and
// non-reversible identifiers.
package whatsapphttp

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/angrosist/demo/internal/channels/whatsapp"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// maxBodyBytes caps the inbound webhook body. Meta payloads are small; this
// blocks a memory-exhaustion attempt on the public route.
const maxBodyBytes = 256 << 10 // 256 KB

// ConversationResolver resolves (or creates) the conversation for a WhatsApp
// sender phone. The Postgres ConversationRepo satisfies it.
type ConversationResolver interface {
	GetOrCreateByChannelPhone(ctx context.Context, channel, phone, vertical, intent string) (*domain.Conversation, error)
}

// Config carries the webhook's verification secrets, sourced from config/env
// (Secret Manager in prod). AppSecret keys the POST signature check; VerifyToken
// gates the GET handshake.
type Config struct {
	AppSecret   string
	VerifyToken string
}

// Handler serves the WhatsApp webhook routes. It owns no business logic beyond
// verification + normalization + enqueue; the agent turn runs asynchronously in
// the worker.
type Handler struct {
	cfg      Config
	resolver ConversationResolver
	queue    ports.Queue
}

// NewHandler wires the webhook handler from its config, the conversation
// resolver, and the queue. resolver/queue must be non-nil for the POST path.
func NewHandler(cfg Config, resolver ConversationResolver, queue ports.Queue) *Handler {
	return &Handler{cfg: cfg, resolver: resolver, queue: queue}
}

// Verify handles GET /api/webhooks/whatsapp — the Meta subscription handshake.
// When hub.mode=subscribe and hub.verify_token matches the configured token
// (constant-time), it echoes hub.challenge verbatim as text/plain 200; otherwise
// 403.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	mode := q.Get("hub.mode")
	token := q.Get("hub.verify_token")
	challenge := q.Get("hub.challenge")

	if mode == "subscribe" && whatsapp.VerifyChallengeToken(h.cfg.VerifyToken, token) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, challenge)
		return
	}

	log.Printf("whatsapp webhook: verification handshake rejected mode=%q", mode)
	http.Error(w, "forbidden", http.StatusForbidden)
}

// Receive handles POST /api/webhooks/whatsapp — signed inbound events. It reads
// the raw body, verifies X-Hub-Signature-256 BEFORE parsing, parses the user text
// messages, resolves/creates a whatsapp conversation per sender phone, and
// enqueues a turn keyed by the WhatsApp message id (idempotency). It always acks
// 200 on a valid signature, even when there is nothing to process (status
// callbacks), so Meta does not retry.
func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		// Could not read the raw body; cannot verify. Reject without enqueueing.
		log.Printf("whatsapp webhook: read body failed: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Signature FIRST (validate-then-parse). On mismatch: 403, log a security
	// event (no body), do not enqueue.
	if err := whatsapp.VerifySignature(h.cfg.AppSecret, body, r.Header.Get("X-Hub-Signature-256")); err != nil {
		log.Printf("whatsapp webhook: signature_rejected source=whatsapp reason=invalid_signature")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	msgs, err := whatsapp.ParseInbound(body)
	if err != nil {
		// Malformed JSON after a valid signature is a terminal parse error: ack so
		// Meta does not retry an unparseable payload.
		log.Printf("whatsapp webhook: parse failed (terminal): %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process each user text message: resolve/create the conversation, then
	// enqueue a turn. The LLM never runs here (ack-fast contract). Per-message
	// failures are logged and skipped so one bad message does not fail the ack.
	ctx := r.Context()
	for _, m := range msgs {
		conv, err := h.resolver.GetOrCreateByChannelPhone(ctx, whatsapp.ChannelWhatsApp, m.From, domain.DefaultVertical, domain.DefaultIntent)
		if err != nil {
			log.Printf("whatsapp webhook: resolve conversation failed msg_id=%s: %v", m.ID, err)
			continue
		}
		job := ports.TurnJob{
			ConversationID: conv.ID,
			Message:        m.Text,
			ProviderMsgID:  m.ID,
		}
		if err := h.queue.Enqueue(ctx, job); err != nil {
			log.Printf("whatsapp webhook: enqueue failed conversation=%s msg_id=%s: %v", conv.ID, m.ID, err)
			continue
		}
	}

	// Fast-ack: 200 means "received", not "processed".
	w.WriteHeader(http.StatusOK)
}
