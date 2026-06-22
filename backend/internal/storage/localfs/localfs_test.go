package localfs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPut_WritesBytesUnderRoot(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	key := "lead/11111111-1111-1111-1111-111111111111/abc-list.csv"
	want := []byte("a,b,c\n1,2,3\n")

	if err := s.Put(context.Background(), key, "text/csv", bytes.NewReader(want)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	full := filepath.Join(root, filepath.FromSlash(key))
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored bytes = %q; want %q", got, want)
	}
}

func TestDelete_RemovesFileAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	key := "lead/33333333-3333-3333-3333-333333333333/uuid-doc.pdf"

	if err := s.Put(context.Background(), key, "application/pdf", bytes.NewReader([]byte("pdfdata"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	full := filepath.Join(root, filepath.FromSlash(key))
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("file should exist after Put: %v", err)
	}

	// First delete removes the file.
	if err := s.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Fatalf("file should be gone after Delete, stat err = %v", err)
	}

	// Idempotent: deleting a missing key is not an error (retried erasure).
	if err := s.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete (idempotent) returned error: %v", err)
	}

	// A traversal-unsafe key is rejected.
	if err := s.Delete(context.Background(), "../escape"); err == nil {
		t.Fatal("Delete should reject a traversal key")
	}
}

func TestOpen_RoundTrip(t *testing.T) {
	s := New(t.TempDir())
	key := "photo/22222222-2222-2222-2222-222222222222/uuid-p.png"
	want := []byte{0x89, 0x50, 0x4e, 0x47, 0x01, 0x02}

	if err := s.Put(context.Background(), key, "image/png", bytes.NewReader(want)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rc, err := s.Open(key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got := new(bytes.Buffer)
	if _, err := got.ReadFrom(rc); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("round-trip = %q; want %q", got.Bytes(), want)
	}
}

func TestURL_ReturnsServedPath(t *testing.T) {
	s := New(t.TempDir())
	key := "lead/33333333-3333-3333-3333-333333333333/u-file.pdf"
	got := s.URL(key)
	want := "/uploads/" + key
	if got != want {
		t.Fatalf("URL = %q; want %q", got, want)
	}
}

func TestSanitizeKey_RejectsTraversal(t *testing.T) {
	cases := []string{
		"",
		"..",
		"../etc/passwd",
		"lead/../../etc/passwd",
		"/etc/passwd",
		"a\\b",
		"x\x00y",
		"./",
	}
	for _, c := range cases {
		if _, err := SanitizeKey(c); err == nil {
			t.Errorf("SanitizeKey(%q) = nil error; want rejection", c)
		}
	}
}

func TestSanitizeKey_AcceptsSafeKeys(t *testing.T) {
	got, err := SanitizeKey("lead/abc/uuid-file.csv")
	if err != nil {
		t.Fatalf("SanitizeKey: %v", err)
	}
	if got != "lead/abc/uuid-file.csv" {
		t.Fatalf("SanitizeKey = %q; unexpected", got)
	}
}

func TestPut_TraversalKeyRejected(t *testing.T) {
	s := New(t.TempDir())
	err := s.Put(context.Background(), "../escape.txt", "text/plain", strings.NewReader("x"))
	if err == nil {
		t.Fatal("Put with traversal key should fail")
	}
}

func TestPut_DefaultRootWhenEmpty(t *testing.T) {
	s := New("")
	if s.Root() != DefaultRoot {
		t.Fatalf("Root = %q; want %q", s.Root(), DefaultRoot)
	}
}
