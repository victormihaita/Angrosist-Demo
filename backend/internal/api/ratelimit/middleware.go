package ratelimit

import (
	"net/http"
	"strconv"
	"time"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/reqmeta"
)

// RateLimit wraps next with per-client-IP rate limiting. It keys on the client IP
// extracted by reqmeta (X-Forwarded-For left-most, else RemoteAddr) so it is
// correct behind Cloudflare / Cloud Run. On exceed it short-circuits with a 429
// in the structured error envelope plus a Retry-After header; otherwise it calls
// next. CORS preflight (OPTIONS) is never rate-limited so the browser handshake
// always succeeds.
//
// It is intended for the PUBLIC, expensive/abusable routes only (POST /api/chat,
// POST /api/conversations/{id}/photos). Do NOT wrap the SSE stream or authed
// dashboard routes with it.
func RateLimit(l *Limiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Never throttle CORS preflight — it carries no payload and must succeed.
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		// Empty key (no resolvable IP) shares a single bucket; that is conservative
		// and acceptable for an abuse limiter.
		key := reqmeta.ClientIPFromRequest(r)
		allowed, retryAfter := l.Allow(key)
		if !allowed {
			secs := int(retryAfter / time.Second)
			if secs < 1 {
				secs = 1
			}
			httputil.ApplyCORS(w, r)
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			httputil.WriteErrorEnvelope(w, http.StatusTooManyRequests, "RATE_LIMITED",
				"too many requests; please slow down and try again shortly")
			return
		}
		next(w, r)
	}
}
