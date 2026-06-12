package handler

import (
	"net/http"

	httputil "github.com/angrosist/demo/internal/adapters/http"
	"github.com/angrosist/demo/internal/app"
	"github.com/angrosist/demo/internal/domain"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	leads, err := app.GetContainer().Leads.List(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list leads")
		return
	}
	if leads == nil {
		leads = make([]*domain.LeadSummary, 0)
	}

	httputil.WriteJSON(w, http.StatusOK, leads)
}
