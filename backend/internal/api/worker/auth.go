package worker

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
)

// authConfig holds the shared-secret bearer token the worker push endpoint
// verifies. It is threaded in from the composition root (WORKER_AUTH_TOKEN) — the
// handler never reads the environment itself.
type authConfig struct {
	// token is the expected bearer value. Empty means auth is DISABLED (local/dev:
	// the in-process queue calls the processor directly and never hits this HTTP
	// route).
	token string
}

// requireBearer guards next with a constant-time bearer-token check.
//
// Behavior (DELIVERABLE 1):
//   - token SET   — a request whose Authorization header is exactly
//     "Bearer <token>" passes; a missing/malformed/mismatched token is rejected
//     with 401 and a security-event log line (the token is NEVER logged).
//   - token UNSET — every request passes (the in-process local queue path; this
//     HTTP route is not exercised in dev). The composition root logs a single
//     startup warning that the endpoint is unauthenticated.
//
// Production control: at provisioning, Cloud Tasks is configured with an OIDC
// token and Cloud Run ingress is restricted to the queue's service account; that
// OIDC verification is the primary prod control. This shared-secret bearer is the
// portable complement that works before/independently of GCP IAM and in any
// transport that can set an Authorization header (the Cloud Tasks adapter already
// sends it — internal/queue/cloudtasks.go).
func requireBearer(cfg authConfig, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.token == "" {
			next(w, r)
			return
		}
		got := bearerToken(r.Header.Get("Authorization"))
		// Constant-time compare to avoid leaking the token via timing.
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(cfg.token)) != 1 {
			// Security event: log the rejection with the path only — never the token
			// or the supplied value (SECURITY.md §2/§8).
			log.Printf("worker: SECURITY rejected unauthenticated push to %s from %s", r.URL.Path, r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header,
// returning "" when the header is absent or not a bearer credential. The scheme
// match is case-insensitive per RFC 7235.
func bearerToken(header string) string {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
