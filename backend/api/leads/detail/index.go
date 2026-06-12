package handler

import (
	"net/http"
	"strings"

	httputil "github.com/angrosist/demo/pkg/adapters/http"
	"github.com/angrosist/demo/pkg/app"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract last path segment as the lead ID.
	// Vercel maps /api/leads/:id → this handler.
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	id := parts[len(parts)-1]
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "missing lead id")
		return
	}

	lead, err := app.GetContainer().Leads.GetByID(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			httputil.WriteError(w, http.StatusNotFound, "lead not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get lead")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, lead)
}
