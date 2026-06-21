package uploadhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/angrosist/demo/internal/api/authhttp"
	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// --- fakes -----------------------------------------------------------------

type fakeStore struct {
	mu   sync.Mutex
	puts map[string][]byte
}

func newFakeStore() *fakeStore { return &fakeStore{puts: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key string, _ string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.puts[key] = b
	f.mu.Unlock()
	return nil
}
func (f *fakeStore) URL(key string) string { return "/uploads/" + key }
func (f *fakeStore) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "/uploads/" + key, nil
}

var _ ports.FileStore = (*fakeStore)(nil)

type fakeDocRepo struct {
	mu      sync.Mutex
	created []*domain.Document
}

func (f *fakeDocRepo) Create(_ context.Context, d *domain.Document) error {
	d.ID = "doc-1"
	f.mu.Lock()
	f.created = append(f.created, d)
	f.mu.Unlock()
	return nil
}
func (f *fakeDocRepo) ListByOwner(context.Context, string, string) ([]*domain.Document, error) {
	return f.created, nil
}

var _ ports.DocumentRepo = (*fakeDocRepo)(nil)

// --- helpers ---------------------------------------------------------------

const validOwnerID = "11111111-1111-1111-1111-111111111111"

// buildMultipart assembles a multipart body with the given form fields and a file
// part (omitted when filename is empty).
func buildMultipart(t *testing.T, fields map[string]string, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	if filename != "" {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return buf, mw.FormDataContentType()
}

// pdfBytes returns bytes that http.DetectContentType sniffs as application/pdf.
func pdfBytes() []byte { return []byte("%PDF-1.4\n%binary stuff\n") }

// pngBytes returns a minimal PNG signature so the sniffer returns image/png.
func pngBytes() []byte {
	return []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
}

func doUpload(t *testing.T, svc *Service, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	svc.Upload(rec, req)
	return rec
}

// --- tests -----------------------------------------------------------------

func TestUpload_Success(t *testing.T) {
	store := newFakeStore()
	docs := &fakeDocRepo{}
	svc := NewService(store, docs, 0)

	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   validOwnerID,
		"kind":       "product_list",
	}, "products.pdf", pdfBytes())

	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != "doc-1" || resp.Key == "" || resp.URL == "" {
		t.Fatalf("response = %+v; want populated id/key/url", resp)
	}
	if !strings.HasPrefix(resp.Key, "lead/"+validOwnerID+"/") {
		t.Fatalf("key %q not namespaced by owner", resp.Key)
	}
	if len(docs.created) != 1 {
		t.Fatalf("documents created = %d; want 1", len(docs.created))
	}
	if got := docs.created[0]; got.Mime != "application/pdf" || got.Kind != "product_list" {
		t.Fatalf("document = %+v; unexpected mime/kind", got)
	}
	if !bytes.Equal(store.puts[resp.Key], pdfBytes()) {
		t.Fatalf("stored bytes mismatch")
	}
}

func TestUpload_PhotoSuccess(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)
	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "listing",
		"owner_id":   validOwnerID,
		"kind":       "photo",
	}, "pic.png", pngBytes())
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpload_Oversize(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 64) // 64-byte cap
	big := bytes.Repeat([]byte("A"), 4096)
	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   validOwnerID,
		"kind":       "product_list",
	}, "big.pdf", append(pdfBytes(), big...))
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 413/400", rec.Code)
	}
}

func TestUpload_DisallowedMIME(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)
	// A PNG uploaded under product_list with a .pdf extension: extension passes,
	// but the sniffed image/png is not allowed for product_list.
	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   validOwnerID,
		"kind":       "product_list",
	}, "list.pdf", pngBytes())
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestUpload_DisallowedKind(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)
	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   validOwnerID,
		"kind":       "malware",
	}, "x.pdf", pdfBytes())
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestUpload_DisallowedExtension(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)
	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   validOwnerID,
		"kind":       "photo",
	}, "evil.exe", pngBytes())
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestUpload_MissingOwnerFields(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)
	body, ct := buildMultipart(t, map[string]string{
		"kind": "product_list",
	}, "list.pdf", pdfBytes())
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestUpload_BadOwnerID(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)
	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   "not-a-uuid",
		"kind":       "product_list",
	}, "list.pdf", pdfBytes())
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestUpload_MissingFile(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)
	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   validOwnerID,
		"kind":       "product_list",
	}, "", nil)
	rec := doUpload(t, svc, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

// TestUpload_NoAuthToken asserts the route behind the staff bearer-token
// middleware rejects an unauthenticated request before the handler runs.
func TestUpload_NoAuthToken(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeDocRepo{}, 0)

	tokens, err := auth.NewTokenIssuer("test-secret-not-a-real-key")
	if err != nil {
		t.Fatalf("token issuer: %v", err)
	}
	authn := authhttp.NewAuthenticator(tokens, stubUserRepo{})
	guarded := authn.Require(svc.Upload)

	body, ct := buildMultipart(t, map[string]string{
		"owner_type": "lead",
		"owner_id":   validOwnerID,
		"kind":       "product_list",
	}, "list.pdf", pdfBytes())
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	guarded(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
}

type stubUserRepo struct{}

func (stubUserRepo) GetByID(context.Context, string) (*domain.User, error) {
	return &domain.User{ID: "u1"}, nil
}
func (stubUserRepo) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, ports.ErrUserNotFound
}
func (stubUserRepo) Create(context.Context, *domain.User) error        { return nil }
func (stubUserRepo) List(context.Context) ([]*domain.User, error)      { return nil, nil }
func (stubUserRepo) UpsertByEmail(context.Context, *domain.User) error { return nil }
