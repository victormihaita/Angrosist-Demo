package handler

import (
	"encoding/json"
	"net/http"

	httputil "github.com/angrosist/demo/pkg/adapters/http"
	"github.com/angrosist/demo/pkg/app"
	"github.com/angrosist/demo/pkg/usecases"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req usecases.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		httputil.WriteError(w, http.StatusBadRequest, "message is required")
		return
	}

	resp, err := app.GetContainer().Chat.RunTurn(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "agent error: "+err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}
