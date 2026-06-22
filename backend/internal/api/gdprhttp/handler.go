// Package gdprhttp is the admin-only HTTP surface for GDPR data-subject rights —
// today the right to erasure (POST /api/gdpr/erasure). The route is mounted behind
// the admin RBAC middleware (authhttp.RequireRole admin) in cmd/server, matching
// the RBAC matrix (SECURITY.md §3: erasure is admin-only). The handler validates
// input, drives the ErasureService use-case, and returns the structured erasure
// report or a structured error envelope. No PII is logged.
package gdprhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/angrosist/demo/internal/api/authhttp"
	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// maxBodyBytes caps the erasure request body; it carries only an id or an email.
const maxBodyBytes = 8 << 10 // 8 KiB

// Eraser is the use-case the handler drives. *usecases.ErasureService satisfies it.
type Eraser interface {
	EraseByContactID(ctx context.Context, actorID, contactID string) (domain.ErasureReport, error)
	EraseByEmail(ctx context.Context, actorID, email string) (domain.ErasureReport, error)
}

// Service wires the erasure use-case into HTTP. Routes are registered by
// cmd/server behind the admin RBAC guard.
type Service struct {
	eraser Eraser
}

// NewService constructs the GDPR HTTP service.
func NewService(eraser Eraser) *Service { return &Service{eraser: eraser} }

// erasureRequest is the POST /api/gdpr/erasure body. Exactly one of ContactID /
// Email must be supplied.
type erasureRequest struct {
	ContactID string `json:"contact_id,omitempty"`
	Email     string `json:"email,omitempty"`
}

// Erasure handles POST /api/gdpr/erasure. It is mounted admin-only. It accepts
// {contact_id} or {email} (exactly one), runs the cascade, and returns the
// erasure report (counts only). Unknown subject -> 404; bad input -> 400.
func (s *Service) Erasure(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteErrorEnvelope(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req erasureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	req.ContactID = strings.TrimSpace(req.ContactID)
	req.Email = strings.TrimSpace(req.Email)

	// Exactly one selector is required.
	if (req.ContactID == "") == (req.Email == "") {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"provide exactly one of contact_id or email")
		return
	}

	// The admin actor (for the proof audit row) comes from the authenticated user
	// placed in the context by the RBAC middleware.
	actorID := ""
	if u := authhttp.UserFrom(r.Context()); u != nil {
		actorID = u.ID
	}

	var (
		report domain.ErasureReport
		err    error
	)
	if req.ContactID != "" {
		report, err = s.eraser.EraseByContactID(r.Context(), actorID, req.ContactID)
	} else {
		report, err = s.eraser.EraseByEmail(r.Context(), actorID, req.Email)
	}
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			httputil.WriteErrorEnvelope(w, http.StatusNotFound, "NOT_FOUND", "no such data subject")
			return
		}
		// No PII / internal detail in the client message.
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "ERASURE_FAILED", "erasure failed")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, report)
}
