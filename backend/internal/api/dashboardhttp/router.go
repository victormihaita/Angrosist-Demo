package dashboardhttp

import (
	"net/http"
	"strings"

	httputil "github.com/angrosist/demo/internal/api/httputil"
)

// Guard wraps a handler with the auth/RBAC middleware. cmd/server passes
// Authenticator.Require so every dashboard route requires a staff bearer token.
type Guard func(http.HandlerFunc) http.HandlerFunc

// Register mounts all dashboard data routes on mux behind guard. It uses Go
// 1.22+ method-and-wildcard patterns so path params (lead/company id) are read
// with r.PathValue. Collection routes (leads, companies, handoffs, kpis) and the
// per-id routes all require staff auth — nothing here is admin-only.
func (s *Service) Register(mux *http.ServeMux, guard Guard) {
	// Leads pipeline + per-id detail/offer/assign. The offer mutation is exposed
	// at both /offer (this task's path) and /status (the API_CONTRACT/openapi path)
	// — same handler, so the contract stays honest without breaking either client.
	mux.HandleFunc("GET /api/leads", guard(s.ListLeads))
	mux.HandleFunc("GET /api/leads/{id}", guard(s.handleLeadByID))
	mux.HandleFunc("PATCH /api/leads/{id}/offer", guard(s.handleOffer))
	mux.HandleFunc("PATCH /api/leads/{id}/status", guard(s.handleOffer))
	mux.HandleFunc("POST /api/leads/{id}/assign", guard(s.handleAssign))

	// B2B directory.
	mux.HandleFunc("GET /api/companies", guard(s.ListCompanies))
	mux.HandleFunc("GET /api/companies/{id}", guard(s.handleCompanyByID))

	// Handoff queue (both spellings) + KPIs.
	mux.HandleFunc("GET /api/handoffs", guard(s.ListHandoffs))
	mux.HandleFunc("GET /api/handoff", guard(s.ListHandoffs))
	mux.HandleFunc("GET /api/kpis", guard(s.KPIs))

	// Preflight for the cross-origin dashboard SPA on the per-id paths.
	mux.HandleFunc("OPTIONS /api/leads/{id}", optionsOnly)
	mux.HandleFunc("OPTIONS /api/leads/{id}/offer", optionsOnly)
	mux.HandleFunc("OPTIONS /api/leads/{id}/status", optionsOnly)
	mux.HandleFunc("OPTIONS /api/leads/{id}/assign", optionsOnly)
	mux.HandleFunc("OPTIONS /api/companies/{id}", optionsOnly)
}

func optionsOnly(w http.ResponseWriter, r *http.Request) {
	httputil.HandleOptions(w, r)
}

// handleLeadByID dispatches GET /api/leads/{id}.
func (s *Service) handleLeadByID(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "missing lead id")
		return
	}
	s.GetLead(w, r, id)
}

// handleOffer dispatches PATCH /api/leads/{id}/offer.
func (s *Service) handleOffer(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "missing lead id")
		return
	}
	s.UpdateOffer(w, r, id)
}

// handleAssign dispatches POST /api/leads/{id}/assign.
func (s *Service) handleAssign(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "missing lead id")
		return
	}
	s.Assign(w, r, id)
}

// handleCompanyByID dispatches GET /api/companies/{id}.
func (s *Service) handleCompanyByID(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "missing company id")
		return
	}
	s.GetCompany(w, r, id)
}
