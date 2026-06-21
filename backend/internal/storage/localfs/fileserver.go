package localfs

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	httputil "github.com/angrosist/demo/internal/api/httputil"
)

// FileServer returns an http.HandlerFunc that serves objects stored by this Store
// under the GET /uploads/{key...} route. It is for local dev / docker only — in
// prod the dashboard fetches private objects via GCS signed URLs, not this route.
//
// The handler re-applies the same key sanitization as Put, so "../" traversal can
// never resolve a path outside the root. It uses Go 1.22+ wildcard path values:
// register it with mux.HandleFunc("GET /uploads/{key...}", store.FileServer()).
func (s *Store) FileServer() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if httputil.HandleOptions(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			httputil.WriteErrorEnvelope(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}

		key := r.PathValue("key")
		if key == "" {
			// Fallback for routers that don't supply the wildcard: derive from path.
			key = strings.TrimPrefix(r.URL.Path, urlPrefix)
		}

		f, err := s.Open(key)
		if err != nil {
			if errors.Is(err, ErrUnsafeKey) {
				httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid path")
				return
			}
			if errors.Is(err, os.ErrNotExist) {
				httputil.WriteErrorEnvelope(w, http.StatusNotFound, "NOT_FOUND", "file not found")
				return
			}
			httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not read file")
			return
		}
		defer f.Close()

		// Never let the browser sniff a different type; the agent uploads are
		// untrusted user content.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// We don't persist content type on disk; let the file route stay generic
		// and force download to avoid inline execution of user content.
		w.Header().Set("Content-Disposition", "attachment")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, f)
	}
}
