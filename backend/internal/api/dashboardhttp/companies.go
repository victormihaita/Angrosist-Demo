package dashboardhttp

import (
	"errors"
	"net/http"
	"strings"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ListCompanies serves GET /api/companies: one keyset page of the B2B directory
// with optional role/country/q filters, ordered created_at DESC.
func (s *Service) ListCompanies(w http.ResponseWriter, r *http.Request) {
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

	f := domain.CompanyFilter{
		Role:    strings.TrimSpace(q.Get("role")),
		Country: strings.TrimSpace(q.Get("country")),
		Query:   strings.TrimSpace(q.Get("q")),
		Limit:   limit,
		After:   cursor,
	}

	items, err := s.companies.ListPage(r.Context(), f)
	if err != nil {
		internal(w, "could not list companies")
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
		items = []*domain.CompanySummary{}
	}
	httputil.WriteJSON(w, http.StatusOK, collection{
		Data: items,
		Page: page{NextCursor: next, Limit: limit, Count: len(items)},
	})
}

// GetCompany serves GET /api/companies/{id}: the directory detail or 404.
func (s *Service) GetCompany(w http.ResponseWriter, r *http.Request, id string) {
	detail, err := s.companies.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) || isNoRows(err) {
			notFound(w, "company not found")
			return
		}
		internal(w, "could not load company")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, detail)
}
