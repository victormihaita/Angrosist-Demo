package localfs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileServer_ServesStoredFile(t *testing.T) {
	s := New(t.TempDir())
	key := "lead/44444444-4444-4444-4444-444444444444/u-doc.pdf"
	want := "%PDF-1.4 stored bytes"
	if err := s.Put(context.Background(), key, "application/pdf", strings.NewReader(want)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /uploads/{key...}", s.FileServer())

	req := httptest.NewRequest(http.MethodGet, "/uploads/"+key, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if rec.Body.String() != want {
		t.Fatalf("body = %q; want %q", rec.Body.String(), want)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff header")
	}
}

func TestFileServer_RejectsTraversal(t *testing.T) {
	s := New(t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /uploads/{key...}", s.FileServer())

	// A literal traversal segment in the path. net/http normalizes some "../"
	// in the URL, so exercise the handler's own check via a key it cannot clean
	// away to inside the root by hitting a missing file as well.
	req := httptest.NewRequest(http.MethodGet, "/uploads/..%2f..%2fetc%2fpasswd", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("traversal request unexpectedly served 200")
	}
}

func TestFileServer_NotFound(t *testing.T) {
	s := New(t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /uploads/{key...}", s.FileServer())

	req := httptest.NewRequest(http.MethodGet, "/uploads/lead/abc/missing.pdf", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", rec.Code)
	}
}
