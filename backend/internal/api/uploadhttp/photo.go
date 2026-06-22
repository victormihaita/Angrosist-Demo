package uploadhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	httputil "github.com/angrosist/demo/internal/api/httputil"
	"github.com/angrosist/demo/internal/convtoken"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
	"github.com/google/uuid"
)

// conversationTokenHeader carries the conversation-ownership token on a photo
// upload (preferred over the "token" form field). See SECURITY.md §1.1.
const conversationTokenHeader = "X-Conversation-Token"

// photoKind is the documents.kind used for seller photos. The PalletClearance
// seller-photo blocking gate counts documents of this kind for the conversation.
const photoKind = "photo"

// photoOwnerType is the polymorphic owner_type the public widget upload binds a
// seller photo to (the conversation). The seller submit later re-points these to
// the durable listing.
const photoOwnerType = "conversation"

// defaultMaxPhotosPerConversation caps how many photos a single (public,
// unauthenticated) conversation may upload, limiting abuse when
// MAX_PHOTOS_PER_CONVERSATION is unset.
const defaultMaxPhotosPerConversation = 10

// SellerConversationGuard reports whether a conversation exists AND is a
// PalletClearance SELLER conversation (vertical=palletclearance, intent=sell).
// The public photo endpoint depends on this narrow port so it can refuse arbitrary
// public writes to non-seller conversations without importing a repository. It is
// satisfied by a tiny adapter over ConversationRepo (see PhotoService wiring).
type SellerConversationGuard interface {
	// IsSellerConversation returns (true, nil) when id names an existing
	// palletclearance/sell conversation, (false, nil) when it exists but is not a
	// seller conversation, and (false, ports.ErrNotFound) when no such conversation
	// exists. Any other error is an infrastructure failure.
	IsSellerConversation(ctx context.Context, id string) (bool, error)
}

// PhotoService serves the PUBLIC, conversation-scoped seller-photo upload
// (POST /api/conversations/{id}/photos). It is UNAUTHENTICATED because the widget
// is public, but tightly scoped: it writes only to existing PalletClearance seller
// conversations, accepts only sniffed image MIME types, caps file size and the
// per-conversation photo count, and never lets the client decide the storage key.
//
// Hexagonal: it depends only on the FileStore + DocumentRepo ports and the
// SellerConversationGuard seam — no infrastructure leaks in. The widget
// seller-photo UI that calls this endpoint is part B (frontend).
type PhotoService struct {
	store     ports.FileStore
	docs      ports.DocumentRepo
	guard     SellerConversationGuard
	tokens    *convtoken.Issuer
	maxBytes  int64
	maxPhotos int
}

// NewPhotoService wires the public photo upload service. maxBytes <= 0 falls back
// to defaultMaxUploadBytes; maxPhotos <= 0 falls back to
// defaultMaxPhotosPerConversation. All ports must be non-nil. tokens verifies the
// conversation-ownership token the widget sends (SECURITY.md §1.1) so a guessed
// conversation id cannot be used to upload to another visitor's conversation.
func NewPhotoService(store ports.FileStore, docs ports.DocumentRepo, guard SellerConversationGuard, tokens *convtoken.Issuer, maxBytes int64, maxPhotos int) *PhotoService {
	if maxBytes <= 0 {
		maxBytes = defaultMaxUploadBytes
	}
	if maxPhotos <= 0 {
		maxPhotos = defaultMaxPhotosPerConversation
	}
	return &PhotoService{store: store, docs: docs, guard: guard, tokens: tokens, maxBytes: maxBytes, maxPhotos: maxPhotos}
}

// Upload handles POST /api/conversations/{id}/photos (multipart/form-data, field
// "file"). It validates the conversation scope, the sniffed image MIME, the size
// cap and the per-conversation photo cap, stores the bytes via the FileStore under
// a server-generated key, records a documents row (owner_type='conversation',
// kind='photo'), and returns {id, key, url}. CORS is applied for the public widget.
func (s *PhotoService) Upload(w http.ResponseWriter, r *http.Request) {
	if httputil.HandleOptions(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteErrorEnvelope(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	convID := r.PathValue("id")
	if _, err := uuid.Parse(convID); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED", "conversation id must be a UUID")
		return
	}

	// Ownership check (SECURITY.md §1.1): the upload must carry the conversation's
	// token via the X-Conversation-Token header (preferred) or a "token" form
	// field. The header is checked up front; the form-field fallback is verified
	// after the multipart parse below. This is IN ADDITION to the seller-scope
	// guard — a guessed conversation id alone is not enough to upload.
	headerToken := r.Header.Get(conversationTokenHeader)

	// Scope guard: only an existing PalletClearance seller conversation may receive
	// public photo uploads. Anything else is 404 (unknown) / 400 (wrong scope) so
	// the public endpoint cannot be used to write to arbitrary conversations.
	ok, err := s.guard.IsSellerConversation(r.Context(), convID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			httputil.WriteErrorEnvelope(w, http.StatusNotFound, "NOT_FOUND", "conversation not found")
			return
		}
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not resolve conversation")
		return
	}
	if !ok {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"photo upload is only allowed for PalletClearance seller conversations")
		return
	}

	// Per-conversation photo cap (abuse limit) — checked before reading the body.
	count, err := s.docs.CountByOwnerKind(r.Context(), photoOwnerType, convID, photoKind)
	if err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not count photos")
		return
	}
	if count >= s.maxPhotos {
		httputil.WriteErrorEnvelope(w, http.StatusConflict, "PHOTO_LIMIT_REACHED",
			fmt.Sprintf("at most %d photos are allowed per conversation", s.maxPhotos))
		return
	}

	// Cap the request body before parsing (slack for the multipart envelope).
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBytes+(1<<16))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
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

	// Verify the conversation-ownership token now that the form is parsed: header
	// (preferred) else the "token" form field. Done before storing any bytes.
	token := headerToken
	if token == "" {
		token = r.FormValue("token")
	}
	if !s.tokens.Verify(convID, token) {
		httputil.WriteErrorEnvelope(w, http.StatusForbidden, "FORBIDDEN",
			"missing or invalid conversation token")
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
	if !extAllowedForKind(photoKind, filename) {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"file extension not allowed",
			httputil.ErrorDetail{Field: "file", Issue: "disallowed extension"})
		return
	}

	// Sniff the content type from the bytes (magic-byte detection); the photo kind
	// allowlist permits only image/jpeg, image/png, image/webp.
	head := make([]byte, sniffLen)
	n, _ := file.Read(head)
	head = head[:n]
	mime := http.DetectContentType(head)
	if !mimeAllowedForKind(photoKind, mime) {
		httputil.WriteErrorEnvelope(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"file content type not allowed",
			httputil.ErrorDetail{Field: "file", Issue: "disallowed content type: " + mime})
		return
	}

	// Server-generated, namespaced key — the client filename never decides the path
	// (SECURITY §1.6 path-traversal guard).
	key := fmt.Sprintf("%s/%s/%s-%s", photoOwnerType, convID, uuid.NewString(), filename)

	body := newRewindReader(head, file)
	if err := s.store.Put(r.Context(), key, mime, body); err != nil {
		httputil.WriteErrorEnvelope(w, http.StatusInternalServerError, "INTERNAL", "could not store file")
		return
	}

	doc := &domain.Document{
		OwnerType: photoOwnerType,
		OwnerID:   convID,
		Kind:      photoKind,
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
