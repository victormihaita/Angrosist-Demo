// Package dashboardhttp implements the authenticated dashboard data endpoints
// (leads pipeline, lead detail, offer tracking, assignment, B2B directory,
// handoff queue, KPIs). Handlers depend only on use-cases and ports; no SQL or
// vendor SDK leaks in here. Every route is mounted behind the auth middleware in
// cmd/server; this package assumes the caller is already authenticated and reads
// the actor via authhttp.UserFrom.
package dashboardhttp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/angrosist/demo/internal/domain"
)

const (
	defaultLimit = 25
	maxLimit     = 100
)

// page is the §4 pagination envelope embedded in every collection response.
type page struct {
	NextCursor *string `json:"next_cursor"`
	Limit      int     `json:"limit"`
	Count      int     `json:"count"`
}

// collection is the standard collection response: data + page (API_CONTRACT §4).
type collection struct {
	Data any  `json:"data"`
	Page page `json:"page"`
}

// parseLimit reads and clamps the `limit` query param to [1, maxLimit]; absent or
// out-of-range values clamp (never error) per the contract.
func parseLimit(r *http.Request) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultLimit
	}
	if n > maxLimit {
		return maxLimit
	}
	return n
}

// encodeCursor renders a keyset position as an opaque base64url JSON token.
func encodeCursor(createdAt time.Time, id string) string {
	b, _ := json.Marshal(domain.Cursor{CreatedAt: createdAt, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor parses the `cursor` query param into a keyset position. An empty
// cursor returns (nil, true) — the first page. A malformed cursor returns
// (nil, false) so the handler can emit 400 VALIDATION_FAILED.
func decodeCursor(r *http.Request) (*domain.Cursor, bool) {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return nil, true
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	var c domain.Cursor
	if err := json.Unmarshal(b, &c); err != nil || c.ID == "" || c.CreatedAt.IsZero() {
		return nil, false
	}
	return &c, true
}
