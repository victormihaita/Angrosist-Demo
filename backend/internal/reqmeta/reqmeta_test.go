package reqmeta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP_RoundTrip(t *testing.T) {
	ctx := WithClientIP(context.Background(), "198.51.100.4")
	if got := ClientIP(ctx); got != "198.51.100.4" {
		t.Fatalf("ClientIP = %q, want 198.51.100.4", got)
	}
	// Absent value -> empty (the WhatsApp path).
	if got := ClientIP(context.Background()); got != "" {
		t.Fatalf("ClientIP (absent) = %q, want empty", got)
	}
}

func TestClientIPFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{"xff left-most", "203.0.113.1, 70.41.3.18, 150.172.238.178", "10.0.0.1:5555", "203.0.113.1"},
		{"remoteaddr fallback", "", "192.0.2.33:443", "192.0.2.33"},
		{"garbage xff falls back", "not-an-ip", "192.0.2.44:80", "192.0.2.44"},
		{"ipv6 remoteaddr", "", "[2001:db8::1]:8080", "2001:db8::1"},
		{"none", "", "garbage", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/chat", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := ClientIPFromRequest(r); got != tt.want {
				t.Fatalf("ClientIPFromRequest = %q, want %q", got, tt.want)
			}
		})
	}
}
