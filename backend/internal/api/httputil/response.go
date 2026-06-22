package httputil

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
)

// WriteJSON writes v as JSON with the given status. CORS is applied separately
// (per request, origin-aware) via ApplyCORS / HandleOptions — call one of those
// at the top of every handler.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// WriteError writes a JSON error body with the given status using the legacy
// demo shape {"error":"<string>"}. Prefer WriteErrorEnvelope for new endpoints
// (API_CONTRACT §3).
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ErrorDetail is one field-level validation issue in the structured envelope.
type ErrorDetail struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// errorBody is the structured error envelope from API_CONTRACT §3:
// {"error":{"code","message","details?"}}.
type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
}

// WriteErrorEnvelope writes the structured error envelope. code is a stable
// SCREAMING_SNAKE_CASE machine-readable identifier; message is human-readable and
// must contain no secrets or PII.
func WriteErrorEnvelope(w http.ResponseWriter, status int, code, message string, details ...ErrorDetail) {
	WriteJSON(w, status, errorBody{Error: errorPayload{Code: code, Message: message, Details: details}})
}

// ---- CORS (env-driven, origin-aware) --------------------------------------

var (
	originsOnce sync.Once
	origins     []string // parsed CORS_ALLOWED_ORIGINS; ["*"] means allow any
)

// allowedOrigins parses CORS_ALLOWED_ORIGINS (comma-separated). It defaults to
// "*" so the public embeddable chat widget keeps working from any partner site;
// production should set an explicit allowlist for the dashboard origins.
// Nothing is hardcoded — the value comes from the environment (Hard Rule #1).
func allowedOrigins() []string {
	originsOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
		if raw == "" {
			raw = "*"
		}
		for _, o := range strings.Split(raw, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	})
	return origins
}

// resolveCORSOrigin decides the Access-Control-Allow-Origin value for a request
// origin against an allowlist. Pure (no env, no I/O) so it is unit-testable.
//   - allowlist ["*"]            -> ("*", vary=false)
//   - requestOrigin in allowlist -> (requestOrigin, vary=true)
//   - otherwise                  -> ("", vary=true)  (no header; browser blocks)
func resolveCORSOrigin(allowed []string, requestOrigin string) (value string, vary bool) {
	if len(allowed) == 1 && allowed[0] == "*" {
		return "*", false
	}
	for _, o := range allowed {
		if o == requestOrigin {
			return requestOrigin, true
		}
	}
	return "", true
}

// ApplyCORS sets CORS response headers for the request's Origin against the
// configured allowlist. With "*" it echoes a wildcard; otherwise it echoes the
// request Origin only when it is allowed (and sets Vary: Origin).
func ApplyCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Conversation-Token")

	value, vary := resolveCORSOrigin(allowedOrigins(), r.Header.Get("Origin"))
	if vary {
		w.Header().Add("Vary", "Origin")
	}
	if value != "" {
		w.Header().Set("Access-Control-Allow-Origin", value)
	}
}

// HandleOptions applies CORS and, for preflight OPTIONS requests, writes 204 and
// returns true so the caller can stop. Call it at the top of every handler.
func HandleOptions(w http.ResponseWriter, r *http.Request) bool {
	ApplyCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
