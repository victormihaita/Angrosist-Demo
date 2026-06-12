package handler

import (
	"context"
	"net/http"
	"time"

	httputil "github.com/angrosist/demo/internal/adapters/http"
	"github.com/angrosist/demo/internal/app"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}

	app.Init()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	dbOK := app.GetContainer().DB.Ping(ctx) == nil
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true, "db": dbOK})
}
