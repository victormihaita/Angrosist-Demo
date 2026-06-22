package gdprhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angrosist/demo/internal/api/authhttp"
	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// fakeEraser records the calls and returns a canned report / error.
type fakeEraser struct {
	byID    map[string]domain.ErasureReport
	byEmail map[string]domain.ErasureReport
	lastID  string
	actor   string
}

func (f *fakeEraser) EraseByContactID(_ context.Context, actorID, contactID string) (domain.ErasureReport, error) {
	f.actor = actorID
	f.lastID = contactID
	if r, ok := f.byID[contactID]; ok {
		return r, nil
	}
	return domain.ErasureReport{}, ports.ErrNotFound
}

func (f *fakeEraser) EraseByEmail(_ context.Context, actorID, email string) (domain.ErasureReport, error) {
	f.actor = actorID
	if r, ok := f.byEmail[email]; ok {
		return r, nil
	}
	return domain.ErasureReport{}, ports.ErrNotFound
}

// mockUserRepo is a minimal ports.UserRepo for the RBAC middleware.
type mockUserRepo struct{ byID map[string]*domain.User }

func (m *mockUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := m.byID[id]; ok {
		return u, nil
	}
	return nil, ports.ErrUserNotFound
}
func (m *mockUserRepo) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, ports.ErrUserNotFound
}
func (m *mockUserRepo) Create(context.Context, *domain.User) error        { return nil }
func (m *mockUserRepo) List(context.Context) ([]*domain.User, error)      { return nil, nil }
func (m *mockUserRepo) UpsertByEmail(context.Context, *domain.User) error { return nil }

func newGuardedHandler(t *testing.T, eraser Eraser, users ...*domain.User) (http.HandlerFunc, *auth.TokenIssuer) {
	t.Helper()
	tokens, err := auth.NewTokenIssuer("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockUserRepo{byID: map[string]*domain.User{}}
	for _, u := range users {
		repo.byID[u.ID] = u
	}
	authn := authhttp.NewAuthenticator(tokens, repo)
	svc := NewService(eraser)
	return authn.RequireRole(domain.RoleAdmin, svc.Erasure), tokens
}

func TestErasure_AdminByContactID(t *testing.T) {
	admin := &domain.User{ID: "a1", Email: "admin@x.io", Role: domain.RoleAdmin}
	eraser := &fakeEraser{byID: map[string]domain.ErasureReport{
		"c-1": {ContactID: "c-1", LeadsDeleted: 2, MessagesDeleted: 9, AuditRowsRedacted: 3},
	}}
	h, tokens := newGuardedHandler(t, eraser, admin)
	tok, _ := tokens.Issue(admin)

	body, _ := json.Marshal(erasureRequest{ContactID: "c-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/gdpr/erasure", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var report domain.ErasureReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.LeadsDeleted != 2 || report.MessagesDeleted != 9 || report.AuditRowsRedacted != 3 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if eraser.actor != "a1" {
		t.Fatalf("actor id = %q, want a1", eraser.actor)
	}
}

func TestErasure_Unauthenticated401(t *testing.T) {
	eraser := &fakeEraser{}
	h, _ := newGuardedHandler(t, eraser)

	body, _ := json.Marshal(erasureRequest{ContactID: "c-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/gdpr/erasure", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestErasure_StaffForbidden403(t *testing.T) {
	staff := &domain.User{ID: "s1", Email: "staff@x.io", Role: domain.RoleStaff}
	eraser := &fakeEraser{}
	h, tokens := newGuardedHandler(t, eraser, staff)
	tok, _ := tokens.Issue(staff)

	body, _ := json.Marshal(erasureRequest{ContactID: "c-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/gdpr/erasure", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestErasure_UnknownContact404(t *testing.T) {
	admin := &domain.User{ID: "a1", Email: "admin@x.io", Role: domain.RoleAdmin}
	eraser := &fakeEraser{byID: map[string]domain.ErasureReport{}}
	h, tokens := newGuardedHandler(t, eraser, admin)
	tok, _ := tokens.Issue(admin)

	body, _ := json.Marshal(erasureRequest{ContactID: "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/gdpr/erasure", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestErasure_BadInput400(t *testing.T) {
	admin := &domain.User{ID: "a1", Email: "admin@x.io", Role: domain.RoleAdmin}
	h, tokens := newGuardedHandler(t, &fakeEraser{}, admin)
	tok, _ := tokens.Issue(admin)

	// Neither contact_id nor email -> 400.
	for _, body := range []erasureRequest{
		{},
		{ContactID: "c-1", Email: "a@b.io"}, // both -> 400
	} {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/gdpr/erasure", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+tok)
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 for body %+v", rr.Code, body)
		}
	}
}
