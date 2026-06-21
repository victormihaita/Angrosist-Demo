package authhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// mockUserRepo is an in-memory ports.UserRepo for handler/middleware tests.
type mockUserRepo struct {
	byID    map[string]*domain.User
	byEmail map[string]*domain.User
}

func newMockRepo(users ...*domain.User) *mockUserRepo {
	m := &mockUserRepo{byID: map[string]*domain.User{}, byEmail: map[string]*domain.User{}}
	for _, u := range users {
		m.byID[u.ID] = u
		m.byEmail[u.Email] = u
	}
	return m
}

func (m *mockUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := m.byID[id]; ok {
		return u, nil
	}
	return nil, ports.ErrUserNotFound
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := m.byEmail[email]; ok {
		return u, nil
	}
	return nil, ports.ErrUserNotFound
}

func (m *mockUserRepo) Create(_ context.Context, u *domain.User) error {
	if _, ok := m.byEmail[u.Email]; ok {
		return ports.ErrUserNotFound // stand-in for a conflict; handler maps any err to 409
	}
	u.ID = "id-" + u.Email
	m.byID[u.ID] = u
	m.byEmail[u.Email] = u
	return nil
}

func (m *mockUserRepo) List(_ context.Context) ([]*domain.User, error) {
	out := make([]*domain.User, 0, len(m.byID))
	for _, u := range m.byID {
		out = append(out, u)
	}
	return out, nil
}

func (m *mockUserRepo) UpsertByEmail(_ context.Context, u *domain.User) error {
	u.ID = "id-" + u.Email
	m.byID[u.ID] = u
	m.byEmail[u.Email] = u
	return nil
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func newTestService(t *testing.T, users *mockUserRepo) (*Service, *auth.TokenIssuer) {
	t.Helper()
	tokens, err := auth.NewTokenIssuer("test-signing-secret")
	if err != nil {
		t.Fatal(err)
	}
	return NewService(users, tokens), tokens
}

func TestLogin_Success(t *testing.T) {
	staff := &domain.User{ID: "u1", Email: "staff@example.com", Name: "Staff", Role: domain.RoleStaff, PasswordHash: mustHash(t, "password123")}
	svc, tokens := newTestService(t, newMockRepo(staff))

	body, _ := json.Marshal(loginRequest{Email: "staff@example.com", Password: "password123"})
	rr := httptest.NewRecorder()
	svc.Login(rr, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp loginResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.User.ID != "u1" || resp.Token == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	claims, err := tokens.Verify(resp.Token)
	if err != nil {
		t.Fatalf("returned token does not verify: %v", err)
	}
	if claims.Subject != "u1" {
		t.Fatalf("token sub = %q, want u1", claims.Subject)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	staff := &domain.User{ID: "u1", Email: "staff@example.com", Role: domain.RoleStaff, PasswordHash: mustHash(t, "password123")}
	svc, _ := newTestService(t, newMockRepo(staff))

	body, _ := json.Marshal(loginRequest{Email: "staff@example.com", Password: "wrong"})
	rr := httptest.NewRecorder()
	svc.Login(rr, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestLogin_NullHash(t *testing.T) {
	// User exists but has no password (NULL hash -> empty string) -> generic 401.
	staff := &domain.User{ID: "u1", Email: "sso@example.com", Role: domain.RoleStaff, PasswordHash: ""}
	svc, _ := newTestService(t, newMockRepo(staff))

	body, _ := json.Marshal(loginRequest{Email: "sso@example.com", Password: "anything"})
	rr := httptest.NewRecorder()
	svc.Login(rr, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestLogin_BadInput(t *testing.T) {
	svc, _ := newTestService(t, newMockRepo())
	body, _ := json.Marshal(loginRequest{Email: "not-an-email", Password: ""})
	rr := httptest.NewRecorder()
	svc.Login(rr, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestRequire_NoHeader(t *testing.T) {
	svc, _ := newTestService(t, newMockRepo())
	called := false
	h := svc.Auth.Require(func(http.ResponseWriter, *http.Request) { called = true })

	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if called {
		t.Fatal("handler must not run without a valid token")
	}
}

func TestRequire_ValidToken_HandlerSeesUser(t *testing.T) {
	staff := &domain.User{ID: "u1", Email: "staff@example.com", Role: domain.RoleStaff}
	svc, tokens := newTestService(t, newMockRepo(staff))
	tok, _ := tokens.Issue(staff)

	var seen *domain.User
	h := svc.Auth.Require(func(_ http.ResponseWriter, r *http.Request) { seen = UserFrom(r.Context()) })

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h(rr, req)

	if seen == nil || seen.ID != "u1" {
		t.Fatalf("handler did not see authenticated user: %+v", seen)
	}
}

func TestRBAC_StaffForbidden_AdminAllowed(t *testing.T) {
	staff := &domain.User{ID: "u1", Email: "staff@example.com", Role: domain.RoleStaff}
	admin := &domain.User{ID: "u2", Email: "admin@example.com", Role: domain.RoleAdmin}
	svc, tokens := newTestService(t, newMockRepo(staff, admin))

	staffTok, _ := tokens.Issue(staff)
	adminTok, _ := tokens.Issue(admin)

	guarded := svc.Auth.RequireRole(domain.RoleAdmin, svc.ListUsers)

	// staff -> 403
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+staffTok)
	rr := httptest.NewRecorder()
	guarded(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("staff status = %d, want 403", rr.Code)
	}

	// admin -> 200
	req = httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rr = httptest.NewRecorder()
	guarded(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}
