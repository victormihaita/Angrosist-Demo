package main

import (
	"log"
	"net/http"
	"os"

	chathandler "github.com/angrosist/demo/api/chat"
	healthhandler "github.com/angrosist/demo/api/health"
	streamhandler "github.com/angrosist/demo/api/stream"
	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/app"
	"github.com/angrosist/demo/internal/domain"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", healthhandler.Handler)
	mux.HandleFunc("/api/chat", chathandler.Handler)
	// SSE stream for live agent replies (long-running server only — not Vercel).
	mux.HandleFunc("/api/stream", streamhandler.Handler(app.GetContainer().Broker))

	// Staff/admin auth + RBAC (M3 Epic 3.3): login is public; user management is
	// admin-only behind bearer-token + role middleware.
	authSvc := app.GetContainer().Auth
	mux.HandleFunc("/api/auth/login", authSvc.Login)
	mux.HandleFunc("/api/users", authSvc.Auth.RequireRole(domain.RoleAdmin, authSvc.ListUsers))

	// Authenticated dashboard DATA endpoints (M3 Epic 3.3 part 2): leads pipeline,
	// lead detail, offer tracking, assignment, B2B directory, handoff queue, KPIs.
	// Every route is mounted behind the staff bearer-token middleware. These
	// supersede the unauth demo /api/leads handlers (the Vercel api/ handlers
	// remain for the demo deployment).
	app.GetContainer().Dashboard.Register(mux, authSvc.Auth.Require)

	// Document upload (M3 Epic 3.2): validated multipart upload behind the staff
	// bearer-token middleware. Storage foundation for buyer product lists +
	// (M4) seller photos.
	mux.HandleFunc("POST /api/upload", authSvc.Auth.Require(app.GetContainer().Upload.Upload))
	mux.HandleFunc("OPTIONS /api/upload", func(w http.ResponseWriter, r *http.Request) {
		httputil.HandleOptions(w, r)
	})

	// Local file-serving route so FileStore.URL works in dev/docker. Only wired
	// for the local-filesystem provider; prod serves private objects via GCS
	// signed URLs, not this route. Public (no auth) like the served bytes would be
	// behind a signed URL otherwise; path traversal is blocked in the handler.
	if localStore := app.GetContainer().LocalFS; localStore != nil {
		mux.HandleFunc("GET /uploads/{key...}", localStore.FileServer())
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Backend running on http://localhost:" + port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
