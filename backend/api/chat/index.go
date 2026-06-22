package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/app"
	"github.com/angrosist/demo/internal/reqmeta"
	"github.com/angrosist/demo/internal/usecases"
)

// conversationTokenHeader carries the conversation-ownership token on a continuing
// turn (preferred over the body field). See SECURITY.md §1.1.
const conversationTokenHeader = "X-Conversation-Token"

const (
	maxBodyBytes      = 64 << 10 // 64 KB — chat turns are small; reject oversized payloads
	maxMessageLen     = 4000     // characters
	maxConversationID = 128
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Cap the request body so a malicious/buggy client can't exhaust memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var req usecases.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate every external input (Hard Rule #6).
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		httputil.WriteError(w, http.StatusBadRequest, "message is required")
		return
	}
	if len([]rune(req.Message)) > maxMessageLen {
		httputil.WriteError(w, http.StatusBadRequest, "message is too long")
		return
	}
	if len(req.ConversationID) > maxConversationID {
		httputil.WriteError(w, http.StatusBadRequest, "invalid conversation_id")
		return
	}

	issuer := app.GetContainer().ConvToken

	// Ownership check (SECURITY.md §1.1 Spoofing): when the client continues an
	// existing conversation it MUST present the server-issued token bound to that
	// conversation_id. A first turn (no conversation_id) creates a brand-new
	// conversation and needs no token (none exists yet). This runs BEFORE the turn
	// and any paid LLM call, so a forged/guessed conversation_id is refused cheaply.
	if req.ConversationID != "" {
		token := r.Header.Get(conversationTokenHeader)
		if token == "" {
			token = req.ConversationToken
		}
		if !issuer.Verify(req.ConversationID, token) {
			httputil.WriteErrorEnvelope(w, http.StatusForbidden, "FORBIDDEN",
				"missing or invalid conversation token")
			return
		}
	}

	// When starting a new conversation, validate the optional vertical/intent up
	// front so an unsupported flow is a 400, not a 500. (Omitted => angrosist/buy.)
	if req.ConversationID == "" {
		if _, _, err := usecases.ValidateVerticalIntent(req.Vertical, req.Intent); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid vertical or intent")
			return
		}
	}

	// Thread the client IP down to consent capture (GDPR proof, SECURITY.md §7.1).
	// It is read from X-Forwarded-For / RemoteAddr and carried via the request
	// context to the agent submit, where a newly created contact records it on its
	// consents row. No PII is logged; the IP is stored only.
	ctx := reqmeta.WithClientIP(r.Context(), reqmeta.ClientIPFromRequest(r))

	resp, err := app.GetContainer().Chat.RunTurn(ctx, req)
	if err != nil {
		// Cost cap: a conversation that hit the max-turns limit is refused with a
		// friendly 429 and no LLM call was made. This is expected client behavior,
		// not a server fault.
		if errors.Is(err, usecases.ErrTooManyTurns) {
			httputil.WriteError(w, http.StatusTooManyRequests,
				"this conversation has reached its message limit; please start a new chat")
			return
		}
		// Log the detail server-side; return a generic message so internal error
		// text (DB/LLM/host fragments) never reaches an unauthenticated caller.
		log.Printf("chat: agent turn failed: %v", err)
		httputil.WriteError(w, http.StatusInternalServerError, "agent error")
		return
	}

	// Issue the ownership token for this (possibly newly created) conversation so
	// every response carries the token the client must echo on the next turn.
	resp.ConversationToken = issuer.Issue(resp.ConversationID)

	httputil.WriteJSON(w, http.StatusOK, resp)
}
