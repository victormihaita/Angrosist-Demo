package dashboardhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/angrosist/demo/internal/api/authhttp"
	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/usecases"
)

// maxBodyBytes caps mutation bodies (offer/assign are tiny JSON objects).
const maxBodyBytes = 16 << 10 // 16 KiB

// maxNoteLen bounds the free-text offer note (API_CONTRACT §7: notes ≤ 2000).
const maxNoteLen = 2000

// Service holds the dashboard data handlers' dependencies (use-cases only).
type Service struct {
	leads     *usecases.LeadUseCase
	companies *usecases.CompanyUseCase
}

// NewService wires the dashboard data HTTP service from the use-cases.
func NewService(leads *usecases.LeadUseCase, companies *usecases.CompanyUseCase) *Service {
	return &Service{leads: leads, companies: companies}
}

// ListLeads serves GET /api/leads (P1): one keyset page of the pipeline with
// optional status/vertical/assigned_to/q filters, ordered created_at DESC.
func (s *Service) ListLeads(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	cursor, ok := decodeCursor(r)
	if !ok {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid cursor")
		return
	}
	limit := parseLimit(r)
	q := r.URL.Query()

	f := domain.LeadFilter{
		Status:   strings.TrimSpace(q.Get("status")),
		Vertical: strings.TrimSpace(q.Get("vertical")),
		Query:    strings.TrimSpace(q.Get("q")),
		Limit:    limit,
		After:    cursor,
	}
	if at, present := q["assigned_to"]; present {
		v := strings.TrimSpace(at[0])
		f.AssignedTo = &v // "" filters for unassigned leads
	}

	items, err := s.leads.ListPage(r.Context(), f)
	if err != nil {
		internal(w, "could not list leads")
		return
	}
	writeLeadPage(w, items, limit)
}

// writeLeadPage emits the §4 envelope, trimming the +1 has-next probe row and
// deriving next_cursor from the last returned item's (created_at, id).
func writeLeadPage(w http.ResponseWriter, items []*domain.LeadSummary, limit int) {
	var next *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		next = &c
	}
	if items == nil {
		items = []*domain.LeadSummary{}
	}
	httputil.WriteJSON(w, http.StatusOK, collection{
		Data: items,
		Page: page{NextCursor: next, Limit: limit, Count: len(items)},
	})
}

// GetLead serves GET /api/leads/{id}: the full LeadDetail or 404.
func (s *Service) GetLead(w http.ResponseWriter, r *http.Request, id string) {
	detail, err := s.leads.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) || isNoRows(err) {
			notFound(w, "lead not found")
			return
		}
		internal(w, "could not load lead")
		return
	}
	if detail == nil {
		notFound(w, "lead not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, detail)
}

// offerRequest is the PATCH /api/leads/{id}/offer body.
type offerRequest struct {
	Status *string  `json:"status"`
	Value  *float64 `json:"value"`
	Note   *string  `json:"note"`
}

// UpdateOffer serves PATCH /api/leads/{id}/offer: manual offer tracking. It
// validates status against the lookup, value ≥ 0, and note length, then persists
// and audits via the use-case.
func (s *Service) UpdateOffer(w http.ResponseWriter, r *http.Request, id string) {
	var req offerRequest
	if err := decodeJSON(r, &req); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}

	var details []httputil.ErrorDetail
	if req.Status != nil && strings.TrimSpace(*req.Status) == "" {
		details = append(details, httputil.ErrorDetail{Field: "status", Issue: "must not be empty"})
	}
	if req.Value != nil && *req.Value < 0 {
		details = append(details, httputil.ErrorDetail{Field: "value", Issue: "must be >= 0"})
	}
	if req.Note != nil && len(*req.Note) > maxNoteLen {
		details = append(details, httputil.ErrorDetail{Field: "note", Issue: "max length 2000"})
	}
	if req.Status == nil && req.Value == nil && req.Note == nil {
		details = append(details, httputil.ErrorDetail{Field: "body", Issue: "at least one of status, value, note required"})
	}
	if len(details) > 0 {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid offer payload", details...)
		return
	}

	actor := actorID(r)
	upd := domain.OfferUpdate{Status: req.Status, Value: req.Value, Note: req.Note}
	summary, err := s.leads.UpdateOffer(r.Context(), actor, id, upd)
	switch {
	case err == nil:
		httputil.WriteJSON(w, http.StatusOK, summary)
	case errors.Is(err, usecases.ErrInvalidStatus):
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "unknown lead status",
			httputil.ErrorDetail{Field: "status", Issue: "not a valid status"})
	case errors.Is(err, ports.ErrNotFound) || isNoRows(err):
		notFound(w, "lead not found")
	default:
		internal(w, "could not update offer")
	}
}

// assignRequest is the POST /api/leads/{id}/assign body. UserID nil unassigns.
type assignRequest struct {
	UserID *string `json:"user_id"`
}

// Assign serves POST /api/leads/{id}/assign: set/clear the lead owner.
func (s *Service) Assign(w http.ResponseWriter, r *http.Request, id string) {
	var req assignRequest
	if err := decodeJSON(r, &req); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	if req.UserID != nil && strings.TrimSpace(*req.UserID) == "" {
		req.UserID = nil // empty string == unassign
	}

	actor := actorID(r)
	summary, err := s.leads.Assign(r.Context(), actor, id, req.UserID)
	switch {
	case err == nil:
		httputil.WriteJSON(w, http.StatusOK, summary)
	case errors.Is(err, usecases.ErrUnknownUser):
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "unknown user",
			httputil.ErrorDetail{Field: "user_id", Issue: "no such user"})
	case errors.Is(err, ports.ErrNotFound) || isNoRows(err):
		notFound(w, "lead not found")
	default:
		internal(w, "could not assign lead")
	}
}

// ListHandoffs serves GET /api/handoffs: the human-handoff queue (needs_human).
func (s *Service) ListHandoffs(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	cursor, ok := decodeCursor(r)
	if !ok {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid cursor")
		return
	}
	limit := parseLimit(r)
	items, err := s.leads.Handoffs(r.Context(), domain.LeadFilter{Limit: limit, After: cursor})
	if err != nil {
		internal(w, "could not list handoffs")
		return
	}

	var next *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		next = &c
	}
	if items == nil {
		items = []*domain.HandoffItem{}
	}
	httputil.WriteJSON(w, http.StatusOK, collection{
		Data: items,
		Page: page{NextCursor: next, Limit: limit, Count: len(items)},
	})
}

// KPIs serves GET /api/kpis: aggregate dashboard KPIs.
func (s *Service) KPIs(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	k, err := s.leads.KPIs(r.Context())
	if err != nil {
		internal(w, "could not compute kpis")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, k)
}

// ---- shared helpers -------------------------------------------------------

// actorID returns the authenticated user id from the request context (set by the
// auth middleware). Empty when no user is present (should not happen behind auth).
func actorID(r *http.Request) string {
	if u := authhttp.UserFrom(r.Context()); u != nil {
		return u.ID
	}
	return ""
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func methodNotAllowed(w http.ResponseWriter) {
	httputil.WriteErrorEnvelope(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func notFound(w http.ResponseWriter, msg string) {
	httputil.WriteErrorEnvelope(w, http.StatusNotFound, "NOT_FOUND", msg)
}

func internal(w http.ResponseWriter, msg string) {
	httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", msg)
}

// isNoRows reports whether err is the pgx "no rows" sentinel surfaced through the
// repo (the legacy GetByID returns it raw rather than ports.ErrNotFound).
func isNoRows(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result set")
}
