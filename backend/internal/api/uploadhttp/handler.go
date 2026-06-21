// Package uploadhttp serves the validated multipart document-upload endpoint
// (POST /api/upload). It is the storage foundation for buyer product lists
// (Angrosist) and future seller photos (PalletClearance). The agent-side
// upload_media tool and the PalletClearance blocking photo gate are M4 and are NOT
// built here.
//
// Hexagonal: the handler depends only on the FileStore and DocumentRepo ports —
// no infrastructure leaks in. It is mounted behind the staff bearer-token
// middleware (uploads are dashboard-adjacent); a public widget upload path is
// added in M4 when the seller flow needs it.
package uploadhttp

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
	"github.com/google/uuid"
)

// defaultMaxUploadBytes caps a single uploaded file when MAX_UPLOAD_BYTES is
// unset. ~10 MiB covers product lists and photos without enabling storage abuse.
const defaultMaxUploadBytes int64 = 10 << 20

// sniffLen is the number of leading bytes read to detect content type. 512 is the
// amount http.DetectContentType inspects.
const sniffLen = 512

// Service holds the upload handler's dependencies (ports only) plus the size cap.
type Service struct {
	store    ports.FileStore
	docs     ports.DocumentRepo
	maxBytes int64
}

// NewService wires the upload HTTP service. maxBytes <= 0 falls back to
// defaultMaxUploadBytes. Both ports must be non-nil.
func NewService(store ports.FileStore, docs ports.DocumentRepo, maxBytes int64) *Service {
	if maxBytes <= 0 {
		maxBytes = defaultMaxUploadBytes
	}
	return &Service{store: store, docs: docs, maxBytes: maxBytes}
}

// uploadResponse is the success envelope returned to the dashboard client.
type uploadResponse struct {
	ID  string `json:"id"`
	Key string `json:"key"`
	URL string `json:"url"`
}

// Upload handles POST /api/upload (multipart/form-data). It validates owner_type,
// owner_id, kind, file size and sniffed MIME, stores the bytes via the FileStore,
// records a documents row, and returns {id, key, url}. Mount it behind the staff
// auth middleware.
func (s *Service) Upload(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteErrorEnvelope(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	// Cap the whole request body before parsing so an oversized upload cannot
	// exhaust memory/disk. MaxBytesReader makes ParseMultipartForm fail past the
	// limit. We add a small slack for the multipart envelope (boundaries/headers).
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBytes+(1<<16))

	// Parse with a modest in-memory threshold; larger parts spill to temp files.
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		// MaxBytesReader surfaces an oversized body here.
		if isMaxBytesError(err) {
			httputil.WriteErrorEnvelope(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				fmt.Sprintf("upload exceeds the %d byte limit", s.maxBytes))
			return
		}
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "malformed multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	ownerType := strings.TrimSpace(r.FormValue("owner_type"))
	ownerID := strings.TrimSpace(r.FormValue("owner_id"))
	kind := strings.TrimSpace(r.FormValue("kind"))

	var details []httputil.ErrorDetail
	if !ownerTypeAllow[ownerType] {
		details = append(details, httputil.ErrorDetail{Field: "owner_type", Issue: "unknown owner type"})
	}
	if _, err := uuid.Parse(ownerID); err != nil {
		details = append(details, httputil.ErrorDetail{Field: "owner_id", Issue: "must be a UUID"})
	}
	if !allowedKind(kind) {
		details = append(details, httputil.ErrorDetail{Field: "kind", Issue: "unknown document kind"})
	}
	if len(details) > 0 {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid upload fields", details...)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "missing file part")
		return
	}
	defer file.Close()

	if header.Size > s.maxBytes {
		httputil.WriteErrorEnvelope(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
			fmt.Sprintf("file exceeds the %d byte limit", s.maxBytes))
		return
	}

	filename := sanitizeFilename(header.Filename)
	if filename == "" {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid filename")
		return
	}
	if !extAllowedForKind(kind, filename) {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"file extension not allowed for this kind",
			httputil.ErrorDetail{Field: "file", Issue: "disallowed extension"})
		return
	}

	// Sniff the content type from the bytes (magic-byte detection), never trusting
	// the client-declared Content-Type header alone.
	head := make([]byte, sniffLen)
	n, _ := file.Read(head)
	head = head[:n]
	mime := http.DetectContentType(head)
	if !mimeAllowedForKind(kind, mime) {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"file content type not allowed for this kind",
			httputil.ErrorDetail{Field: "file", Issue: "disallowed content type: " + mime})
		return
	}

	// Build a safe, namespaced storage key. The server generates the key — the
	// client filename never decides the path (SECURITY §1.6 path-traversal guard).
	key := fmt.Sprintf("%s/%s/%s-%s", ownerType, ownerID, uuid.NewString(), filename)

	// Stream the (already-read head) + the rest to storage.
	body := newRewindReader(head, file)
	if err := s.store.Put(r.Context(), key, mime, body); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not store file")
		return
	}

	doc := &domain.Document{
		OwnerType: ownerType,
		OwnerID:   ownerID,
		Kind:      kind,
		GCSKey:    key,
		Mime:      mime,
		SizeBytes: header.Size,
	}
	if err := s.docs.Create(r.Context(), doc); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not record document")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, uploadResponse{
		ID:  doc.ID,
		Key: key,
		URL: s.store.URL(key),
	})
}

// sanitizeFilename reduces a client filename to a safe basename with no path
// separators, traversal segments, or control characters. Returns "" when nothing
// usable remains.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Drop any directory components the client may have included.
	name = filepath.Base(filepath.FromSlash(name))
	name = strings.ReplaceAll(name, "\\", "")
	if name == "." || name == ".." {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return ""
	}
	return out
}

// isMaxBytesError reports whether err comes from http.MaxBytesReader tripping.
func isMaxBytesError(err error) bool {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return true
	}
	// ParseMultipartForm can wrap the limit error in a plain text error too.
	return err != nil && strings.Contains(err.Error(), "request body too large")
}
