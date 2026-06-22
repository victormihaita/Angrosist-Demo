// Package reqmeta carries request-scoped metadata (today: the client IP) down
// through the call graph via context.Context, so inner layers can record it
// without depending on net/http or reaching for globals. The HTTP boundary sets
// the value; use-cases / the agent submit read it.
//
// Only non-PII, request-derived facts belong here. The client IP is GDPR proof
// data for consent capture (consents.ip, SECURITY.md §7.1) — it is stored, never
// logged.
package reqmeta

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// ctxKey is an unexported context key type so other packages cannot collide.
type ctxKey int

const clientIPKey ctxKey = iota

// WithClientIP returns a copy of ctx carrying the client IP. An empty ip is
// stored as-is (callers reading it then get "").
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

// ClientIP returns the client IP previously stored with WithClientIP, or "" when
// none was set (e.g. the WhatsApp path, which has no HTTP request IP).
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}

// ClientIPFromRequest extracts the best-effort client IP from an HTTP request,
// preferring the left-most entry of X-Forwarded-For (the original client when
// behind Cloudflare / Cloud Run), then falling back to RemoteAddr (host part).
// It returns "" when no usable IP is found. The value is validated as an IP so a
// spoofed/garbage header is dropped rather than stored.
func ClientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Left-most is the original client; subsequent entries are proxies.
		first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return ip.String()
	}
	return ""
}
