// Package authhttp wires the auth/RBAC layer into net/http: it verifies bearer
// JWTs, loads the user, exposes the current user from the request context, and
// enforces role requirements. Handlers depend on the auth package and the
// UserRepo port only — no infrastructure leaks in here.
package authhttp

import (
	"context"
	"net/http"
	"strings"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ctxKey is an unexported context key type so other packages cannot collide.
type ctxKey int

const userCtxKey ctxKey = iota

// Authenticator verifies bearer tokens and loads the authenticated user. It
// composes a token issuer (verify) with the user repository (load), and provides
// middleware plus a context accessor.
type Authenticator struct {
	tokens *auth.TokenIssuer
	users  ports.UserRepo
}

// NewAuthenticator builds an Authenticator from the token issuer and user repo.
func NewAuthenticator(tokens *auth.TokenIssuer, users ports.UserRepo) *Authenticator {
	return &Authenticator{tokens: tokens, users: users}
}

// UserFrom returns the authenticated user stored in ctx by Require, or nil.
func UserFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(userCtxKey).(*domain.User)
	return u
}

// withUser returns a copy of ctx carrying the authenticated user.
func withUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, userCtxKey, u)
}

// bearerToken extracts the token from an "Authorization: Bearer <jwt>" header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// Require wraps next so it only runs for requests carrying a valid bearer token
// whose subject resolves to an existing user. On any failure it writes a 401
// envelope and does not call next. The authenticated user is placed in the
// request context (read it with UserFrom).
func (a *Authenticator) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearerToken(r)
		if tok == "" {
			unauthorized(w)
			return
		}
		claims, err := a.tokens.Verify(tok)
		if err != nil {
			unauthorized(w)
			return
		}
		// Load the live user so role changes / deletions take effect immediately
		// and the handler never trusts the role claim alone.
		u, err := a.users.GetByID(r.Context(), claims.Subject)
		if err != nil || u == nil {
			unauthorized(w)
			return
		}
		next(w, r.WithContext(withUser(r.Context(), u)))
	}
}

// RequireRole wraps next with Require and additionally enforces that the
// authenticated user holds role. A non-matching role yields a 403 envelope.
func (a *Authenticator) RequireRole(role domain.Role, next http.HandlerFunc) http.HandlerFunc {
	return a.Require(func(w http.ResponseWriter, r *http.Request) {
		u := UserFrom(r.Context())
		if u == nil || (role == domain.RoleAdmin && !u.IsAdmin()) || (role != domain.RoleAdmin && u.Role != role) {
			forbidden(w)
			return
		}
		next(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	httputil.WriteErrorEnvelope(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
}

func forbidden(w http.ResponseWriter) {
	httputil.WriteErrorEnvelope(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
}
