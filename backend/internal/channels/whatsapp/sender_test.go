package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSender_Send_RequestShape(t *testing.T) {
	var (
		gotPath   string
		gotAuth   string
		gotCT     string
		gotMethod string
		gotBody   map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.OUT"}]}`))
	}))
	defer srv.Close()

	s := NewSenderWithClient(SenderConfig{
		APIBase:       srv.URL,
		Token:         "tok-123",
		PhoneNumberID: "PNID",
	}, srv.Client())

	if err := s.Send(context.Background(), "40712345678", "Buna ziua"); err != nil {
		t.Fatalf("send: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/PNID/messages" {
		t.Errorf("path = %s, want /PNID/messages", gotPath)
	}
	if gotAuth != "Bearer tok-123" {
		t.Errorf("auth = %q, want Bearer tok-123", gotAuth)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	if gotBody["messaging_product"] != "whatsapp" {
		t.Errorf("messaging_product = %v", gotBody["messaging_product"])
	}
	if gotBody["to"] != "40712345678" {
		t.Errorf("to = %v", gotBody["to"])
	}
	if gotBody["type"] != "text" {
		t.Errorf("type = %v", gotBody["type"])
	}
	text, _ := gotBody["text"].(map[string]any)
	if text == nil || text["body"] != "Buna ziua" {
		t.Errorf("text.body = %v", gotBody["text"])
	}
}

func TestSender_Send_NotConfigured(t *testing.T) {
	s := NewSender(SenderConfig{APIBase: "https://example.test"}) // no token / phone id
	if s.Configured() {
		t.Fatal("expected not configured")
	}
	if err := s.Send(context.Background(), "4071", "x"); err == nil {
		t.Fatal("expected ErrNotConfigured")
	}
}

func TestSender_Send_Non2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	s := NewSenderWithClient(SenderConfig{APIBase: srv.URL, Token: "t", PhoneNumberID: "p"}, srv.Client())
	if err := s.Send(context.Background(), "4071", "x"); err == nil {
		t.Fatal("expected error on non-2xx response")
	}
}

func TestSender_DefaultAPIBase(t *testing.T) {
	s := NewSender(SenderConfig{Token: "t", PhoneNumberID: "p"})
	if s.apiBase != defaultAPIBase {
		t.Fatalf("apiBase = %q, want default %q", s.apiBase, defaultAPIBase)
	}
}
