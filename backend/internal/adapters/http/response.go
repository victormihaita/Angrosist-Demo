package httputil

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	setCORS(w)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

func SetCORSHeaders(w http.ResponseWriter) {
	setCORS(w)
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func HandleOptions(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodOptions {
		setCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
