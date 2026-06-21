package dashboardhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angrosist/demo/internal/api/authhttp"
	"github.com/angrosist/demo/internal/auth"
)

// TestAuthZSmoke_NoToken401 mounts the dashboard routes behind the real auth
// middleware and asserts a representative endpoint rejects an unauthenticated
// request with 401. Full RBAC is covered in the auth package's tests (part 1).
func TestAuthZSmoke_NoToken401(t *testing.T) {
	tokens, err := auth.NewTokenIssuer("test-signing-secret")
	if err != nil {
		t.Fatal(err)
	}
	authn := authhttp.NewAuthenticator(tokens, stubUserRepo{})

	svc := newService(&stubLeadRepo{}, &stubCompanyRepo{})
	mux := http.NewServeMux()
	svc.Register(mux, authn.Require)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/leads", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/leads = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
}
