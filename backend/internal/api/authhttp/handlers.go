package authhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// maxBodyBytes caps auth request bodies; credentials and user records are tiny.
const maxBodyBytes = 16 << 10 // 16 KiB

// Service holds the dependencies for the auth HTTP handlers: the user repository
// (port) and the token issuer. It exposes both the public login handler and the
// admin-only user-management handlers, plus the Authenticator for middleware.
type Service struct {
	users  ports.UserRepo
	tokens *auth.TokenIssuer
	Auth   *Authenticator
}

// NewService wires the auth HTTP service. tokens must be non-nil (the server
// fails fast when JWT_SECRET is unset, so a nil issuer never reaches here).
func NewService(users ports.UserRepo, tokens *auth.TokenIssuer) *Service {
	return &Service{
		users:  users,
		tokens: tokens,
		Auth:   NewAuthenticator(tokens, users),
	}
}

// ---- POST /api/auth/login (public) ---------------------------------------

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string            `json:"token"`
	User  domain.PublicUser `json:"user"`
}

// Login authenticates email+password and returns a signed JWT. Bad credentials,
// an unknown email, or a password-less account all return the same generic 401
// so the caller cannot tell which check failed.
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteErrorEnvelope(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if details := validateLogin(req); len(details) > 0 {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid credentials payload", details...)
		return
	}

	user, err := s.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		// ErrUserNotFound and any lookup error collapse to a generic 401 (timing
		// difference is acceptable here; the goal is not leaking which check failed).
		s.unauthorizedLogin(w)
		return
	}
	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		s.unauthorizedLogin(w)
		return
	}

	token, err := s.tokens.Issue(user)
	if err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not issue token")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, loginResponse{Token: token, User: user.Public()})
}

func (s *Service) unauthorizedLogin(w http.ResponseWriter) {
	httputil.WriteErrorEnvelope(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid email or password")
}

// ---- GET/POST /api/users (admin only) ------------------------------------

// ListUsers returns all users as PublicUser projections. Wire it behind
// Authenticator.RequireRole(RoleAdmin).
func (s *Service) ListUsers(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listUsers(w, r)
	case http.MethodPost:
		s.createUser(w, r)
	default:
		httputil.WriteErrorEnvelope(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (s *Service) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.users.List(r.Context())
	if err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not list users")
		return
	}
	out := make([]domain.PublicUser, 0, len(users))
	for _, u := range users {
		out = append(out, u.Public())
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

type createUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

func (s *Service) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(r, &req); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)
	req.Role = strings.TrimSpace(req.Role)

	details := validateCreateUser(req)
	if len(details) > 0 {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid user payload", details...)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not hash password")
		return
	}

	user := &domain.User{
		Email:        req.Email,
		Name:         req.Name,
		Role:         domain.Role(req.Role),
		PasswordHash: hash,
	}
	if err := s.users.Create(r.Context(), user); err != nil {
		// A duplicate email is the only expected user-facing failure here.
		httputil.WriteErrorEnvelope(w, http.StatusConflict, "CONFLICT", "could not create user")
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, user.Public())
}

// ---- validation ----------------------------------------------------------

func validateLogin(req loginRequest) []httputil.ErrorDetail {
	var d []httputil.ErrorDetail
	if req.Email == "" {
		d = append(d, httputil.ErrorDetail{Field: "email", Issue: "required"})
	} else if _, err := mail.ParseAddress(req.Email); err != nil {
		d = append(d, httputil.ErrorDetail{Field: "email", Issue: "invalid"})
	}
	if req.Password == "" {
		d = append(d, httputil.ErrorDetail{Field: "password", Issue: "required"})
	}
	return d
}

func validateCreateUser(req createUserRequest) []httputil.ErrorDetail {
	var d []httputil.ErrorDetail
	if req.Email == "" {
		d = append(d, httputil.ErrorDetail{Field: "email", Issue: "required"})
	} else if _, err := mail.ParseAddress(req.Email); err != nil {
		d = append(d, httputil.ErrorDetail{Field: "email", Issue: "invalid"})
	}
	if req.Name == "" {
		d = append(d, httputil.ErrorDetail{Field: "name", Issue: "required"})
	}
	switch domain.Role(req.Role) {
	case domain.RoleStaff, domain.RoleAdmin:
	default:
		d = append(d, httputil.ErrorDetail{Field: "role", Issue: "must be staff or admin"})
	}
	if len(req.Password) < 8 {
		d = append(d, httputil.ErrorDetail{Field: "password", Issue: "min length 8"})
	}
	return d
}

// decodeJSON reads a size-capped, strict JSON body into v.
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
