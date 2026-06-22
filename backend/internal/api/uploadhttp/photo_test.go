package uploadhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angrosist/demo/internal/convtoken"
	"github.com/angrosist/demo/internal/ports"
)

// testIssuer is the shared conversation-token issuer for the photo handler tests.
var testIssuer = convtoken.New("photo-test-secret", "")

// validToken returns the correct ownership token for convID under testIssuer.
func validToken(convID string) string { return testIssuer.Issue(convID) }

// --- fakes for the photo endpoint ------------------------------------------

// fakeGuard answers the conversation-scope question deterministically.
type fakeGuard struct {
	seller   bool
	notFound bool
}

func (g fakeGuard) IsSellerConversation(context.Context, string) (bool, error) {
	if g.notFound {
		return false, ports.ErrNotFound
	}
	return g.seller, nil
}

const photoConvID = "22222222-2222-2222-2222-222222222222"

// countingDocRepo extends the basic fake with a configurable existing-photo count
// so the per-conversation cap can be exercised.
type countingDocRepo struct {
	fakeDocRepo
	count int
}

func (c *countingDocRepo) CountByOwnerKind(context.Context, string, string, string) (int, error) {
	return c.count, nil
}

func doPhotoUpload(t *testing.T, svc *PhotoService, convID string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+convID+"/photos", body)
	req.SetPathValue("id", convID)
	req.Header.Set("Content-Type", contentType)
	// Existing tests exercise scope/validation with a valid ownership token; the
	// token-specific tests use doPhotoUploadToken to send their own (or none).
	req.Header.Set("X-Conversation-Token", validToken(convID))
	rec := httptest.NewRecorder()
	svc.Upload(rec, req)
	return rec
}

// doPhotoUploadToken is like doPhotoUpload but sends the given ownership token
// verbatim (empty => no token header), so the token-enforcement tests can drive
// the missing/wrong/correct cases.
func doPhotoUploadToken(t *testing.T, svc *PhotoService, convID, token string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+convID+"/photos", body)
	req.SetPathValue("id", convID)
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("X-Conversation-Token", token)
	}
	rec := httptest.NewRecorder()
	svc.Upload(rec, req)
	return rec
}

// --- tests -----------------------------------------------------------------

func TestPhotoUpload_SellerSuccess(t *testing.T) {
	store := newFakeStore()
	docs := &fakeDocRepo{}
	svc := NewPhotoService(store, docs, fakeGuard{seller: true}, testIssuer, 0, 0)

	body, ct := buildMultipart(t, nil, "pic.png", pngBytes())
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == "" || resp.Key == "" || resp.URL == "" {
		t.Fatalf("response = %+v; want populated id/key/url", resp)
	}
	if !strings.HasPrefix(resp.Key, "conversation/"+photoConvID+"/") {
		t.Fatalf("key %q not scoped to conversation", resp.Key)
	}
	if len(docs.created) != 1 {
		t.Fatalf("documents created = %d; want 1", len(docs.created))
	}
	got := docs.created[0]
	if got.OwnerType != "conversation" || got.OwnerID != photoConvID || got.Kind != "photo" {
		t.Fatalf("document owner/kind wrong: %+v", got)
	}
	if got.Mime != "image/png" {
		t.Fatalf("document mime = %q; want image/png", got.Mime)
	}
}

func TestPhotoUpload_JPEGSuccess(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	body, ct := buildMultipart(t, nil, "pic.jpg", jpegBytes())
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPhotoUpload_NonSellerConversation(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: false}, testIssuer, 0, 0)
	body, ct := buildMultipart(t, nil, "pic.png", pngBytes())
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for non-seller conversation", rec.Code)
	}
}

func TestPhotoUpload_UnknownConversation(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{notFound: true}, testIssuer, 0, 0)
	body, ct := buildMultipart(t, nil, "pic.png", pngBytes())
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for unknown conversation", rec.Code)
	}
}

func TestPhotoUpload_BadConversationID(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	body, ct := buildMultipart(t, nil, "pic.png", pngBytes())
	rec := doPhotoUpload(t, svc, "not-a-uuid", body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for non-UUID id", rec.Code)
	}
}

func TestPhotoUpload_NonImageRejected(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	// A PDF declared with a .png extension: extension passes the cheap check but the
	// sniffed application/pdf is not an allowed image type.
	body, ct := buildMultipart(t, nil, "doc.png", pdfBytes())
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for non-image", rec.Code)
	}
}

func TestPhotoUpload_DisallowedExtension(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	body, ct := buildMultipart(t, nil, "evil.exe", pngBytes())
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for disallowed extension", rec.Code)
	}
}

func TestPhotoUpload_Oversize(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 64, 0)
	big := append(pngBytes(), bytes.Repeat([]byte("A"), 4096)...)
	body, ct := buildMultipart(t, nil, "big.png", big)
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 413/400 for oversize", rec.Code)
	}
}

func TestPhotoUpload_PerConversationCap(t *testing.T) {
	docs := &countingDocRepo{count: 3} // already at the cap
	svc := NewPhotoService(newFakeStore(), docs, fakeGuard{seller: true}, testIssuer, 0, 3)
	body, ct := buildMultipart(t, nil, "pic.png", pngBytes())
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusConflict && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 409/400 when over the per-conversation cap", rec.Code)
	}
}

func TestPhotoUpload_MissingFile(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	body, ct := buildMultipart(t, nil, "", nil)
	rec := doPhotoUpload(t, svc, photoConvID, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for missing file", rec.Code)
	}
}

// TestPhotoUpload_MissingToken asserts an upload to a valid seller conversation
// WITHOUT the ownership token is refused 403 (SECURITY.md §1.1) — the seller scope
// guard alone is not enough.
func TestPhotoUpload_MissingToken(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	body, ct := buildMultipart(t, nil, "pic.png", pngBytes())
	rec := doPhotoUploadToken(t, svc, photoConvID, "", body, ct)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403 for missing token", rec.Code)
	}
}

// TestPhotoUpload_WrongToken asserts a token for a DIFFERENT conversation is
// refused 403 even on a valid seller conversation.
func TestPhotoUpload_WrongToken(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	wrong := testIssuer.Issue("33333333-3333-3333-3333-333333333333")
	body, ct := buildMultipart(t, nil, "pic.png", pngBytes())
	rec := doPhotoUploadToken(t, svc, photoConvID, wrong, body, ct)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403 for wrong token", rec.Code)
	}
}

// TestPhotoUpload_TokenViaFormField asserts the "token" form-field fallback is
// accepted when the header is absent.
func TestPhotoUpload_TokenViaFormField(t *testing.T) {
	svc := NewPhotoService(newFakeStore(), &fakeDocRepo{}, fakeGuard{seller: true}, testIssuer, 0, 0)
	fields := map[string]string{"token": validToken(photoConvID)}
	body, ct := buildMultipart(t, fields, "pic.png", pngBytes())
	rec := doPhotoUploadToken(t, svc, photoConvID, "", body, ct) // no header
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 with token via form field; body=%s", rec.Code, rec.Body.String())
	}
}

// jpegBytes returns bytes that http.DetectContentType sniffs as image/jpeg.
func jpegBytes() []byte {
	return []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 0}
}
