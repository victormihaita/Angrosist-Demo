package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	httputil "github.com/angrosist/demo/pkg/adapters/http"
	"github.com/angrosist/demo/pkg/app"
	"github.com/angrosist/demo/pkg/usecases"
)

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

	resp, err := app.GetContainer().Chat.RunTurn(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "agent error: "+err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}
