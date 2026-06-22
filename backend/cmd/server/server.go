package main

import (
	"net/http"

	chathandler "github.com/angrosist/demo/api/chat"
	healthhandler "github.com/angrosist/demo/api/health"
	streamhandler "github.com/angrosist/demo/api/stream"
	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/api/ratelimit"
	"github.com/angrosist/demo/internal/app"
	"github.com/angrosist/demo/internal/domain"
)

// Router builds the complete API mux from an assembled container and returns it as
// an http.Handler. main() calls it to serve; tests mount the SAME mux against an
// httptest.Server so the black-box E2E exercises the exact production routing.
//
// It registers identical routes to the previous inline main(): the public widget
// API (health/chat/stream), the signature-verified WhatsApp webhook, the auth +
// RBAC surface, the authenticated dashboard data endpoints, document/photo upload,
// the admin-only GDPR erasure endpoint, and (local FileStore only) the dev
// file-serving route. Behavior is unchanged — this is a pure extraction so the
// router is testable.
//
// The public chat/health/stream handlers resolve their dependencies via
// app.GetContainer() internally (the assembled singleton), so c is used for the
// route surfaces that hold their own services. Pass app.GetContainer().
func Router(c *app.Container) http.Handler {
	mux := http.NewServeMux()

	// Per-client-IP rate limiter for the PUBLIC, expensive/abusable routes only
	// (POST /api/chat, POST /api/conversations/{id}/photos). The SSE stream and the
	// authed dashboard routes are intentionally NOT wrapped. Per-instance; multi-
	// instance prod additionally relies on Cloudflare WAF / a Redis-backed limiter
	// behind this same middleware seam.
	rl := c.RateLimiter

	mux.HandleFunc("/api/health", healthhandler.Handler)
	mux.HandleFunc("/api/chat", ratelimit.RateLimit(rl, chathandler.Handler))
	// SSE stream for live agent replies (long-running server only — not Vercel).
	mux.HandleFunc("/api/stream", streamhandler.Handler(c.Broker, c.ConvToken))

	// WhatsApp webhook (M4 part C): PUBLIC routes (no bearer auth) but every POST
	// is authenticated by X-Hub-Signature-256 over the raw body; the GET handshake
	// checks hub.verify_token. The channel is inert until WHATSAPP_* is configured
	// and the Meta number is verified (owner action), but the routes (and signature
	// enforcement) are always live. Path per API_CONTRACT §8.2.
	whatsApp := c.WhatsApp
	mux.HandleFunc("GET /api/webhooks/whatsapp", whatsApp.Verify)
	mux.HandleFunc("POST /api/webhooks/whatsapp", whatsApp.Receive)

	// Staff/admin auth + RBAC (M3 Epic 3.3): login is public; user management is
	// admin-only behind bearer-token + role middleware.
	authSvc := c.Auth
	mux.HandleFunc("/api/auth/login", authSvc.Login)
	mux.HandleFunc("/api/users", authSvc.Auth.RequireRole(domain.RoleAdmin, authSvc.ListUsers))

	// Authenticated dashboard DATA endpoints (M3 Epic 3.3 part 2): leads pipeline,
	// lead detail, offer tracking, assignment, B2B directory, handoff queue, KPIs.
	// Every route is mounted behind the staff bearer-token middleware.
	c.Dashboard.Register(mux, authSvc.Auth.Require)

	// Document upload (M3 Epic 3.2): validated multipart upload behind the staff
	// bearer-token middleware.
	mux.HandleFunc("POST /api/upload", authSvc.Auth.Require(c.Upload.Upload))
	mux.HandleFunc("OPTIONS /api/upload", func(w http.ResponseWriter, r *http.Request) {
		httputil.HandleOptions(w, r)
	})

	// GDPR right-to-erasure (M5): ADMIN-ONLY (RBAC matrix, SECURITY.md §3). Runs the
	// cascade-erasure of a data subject (contact_id|email) and returns the counts
	// report. The audit trail is redacted, not deleted, and company data is preserved.
	gdpr := c.GDPR
	mux.HandleFunc("POST /api/gdpr/erasure", authSvc.Auth.RequireRole(domain.RoleAdmin, gdpr.Erasure))
	mux.HandleFunc("OPTIONS /api/gdpr/erasure", func(w http.ResponseWriter, r *http.Request) {
		httputil.HandleOptions(w, r)
	})

	// Public, conversation-scoped seller-photo upload (M4 PalletClearance): the
	// widget is public, so this route is UNAUTHENTICATED but scoped to existing
	// PalletClearance seller conversations (validated in the handler).
	photos := c.Photos
	mux.HandleFunc("POST /api/conversations/{id}/photos", ratelimit.RateLimit(rl, photos.Upload))
	mux.HandleFunc("OPTIONS /api/conversations/{id}/photos", func(w http.ResponseWriter, r *http.Request) {
		httputil.HandleOptions(w, r)
	})

	// Local file-serving route so FileStore.URL works in dev/docker. Only wired for
	// the local-filesystem provider; prod serves private objects via GCS signed URLs.
	if localStore := c.LocalFS; localStore != nil {
		mux.HandleFunc("GET /uploads/{key...}", localStore.FileServer())
	}

	return mux
}
