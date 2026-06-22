// Package gcs is the (DEFERRED) Google Cloud Storage adapter for the
// ports.FileStore seam. GCS provisioning is deferred together with Cloud Run /
// Terraform, so this package is intentionally a documented stub: it does NOT
// import cloud.google.com/go/storage (that heavy dependency is added at
// provisioning) and every method returns ErrNotConfigured so a misconfiguration
// fails loudly instead of silently dropping uploads.
//
// # Landing the real adapter (at provisioning)
//
// Implement Store against cloud.google.com/go/storage:
//
//   - Put: bucket.Object(key).NewWriter(ctx); set ObjectAttrs.ContentType; stream
//     r; close. Bucket is PRIVATE (SECURITY §1.6: never public).
//   - URL: not used for the private bucket — return SignedURL instead.
//   - SignedURL: storage.SignedURL(bucket, key, &storage.SignedURLOptions{
//     Method: GET, Expires: now+ttl, Scheme: V4}) using the runtime service
//     account (prefer Workload Identity over an exported key file).
//
// Wiring then becomes a one-line switch in internal/app/container.go on
// FILESTORE_PROVIDER=gcs, with the bucket name from config (GCS_BUCKET) — never
// hardcoded. The agent's upload_media tool and the upload endpoint are unchanged
// because they depend only on the FileStore port.
package gcs

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/angrosist/demo/internal/ports"
)

// ErrNotConfigured signals that the GCS adapter is selected but not yet wired to
// a real bucket (provisioning is deferred). It is returned by every method so the
// failure is explicit and never mistaken for success.
var ErrNotConfigured = errors.New("gcs filestore: not configured (provisioning deferred — see internal/storage/gcs)")

// Config holds the values the real adapter will read from internal/config. Kept
// here so the wiring seam already exists; nothing is hardcoded.
type Config struct {
	// Bucket is the private GCS bucket name (from GCS_BUCKET). EU residency: the
	// bucket must live in a europe-* location.
	Bucket string
}

// Store is the placeholder GCS FileStore. Replace its method bodies with real
// GCS calls at provisioning; the type and port satisfaction are already in place.
type Store struct {
	cfg Config
}

// New constructs the placeholder GCS Store from cfg.
func New(cfg Config) *Store { return &Store{cfg: cfg} }

// Put is not yet implemented (provisioning deferred).
func (s *Store) Put(context.Context, string, string, io.Reader) error { return ErrNotConfigured }

// URL is not yet implemented; the prod bucket is private — use SignedURL.
func (s *Store) URL(string) string { return "" }

// SignedURL is not yet implemented (provisioning deferred).
func (s *Store) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

// Delete is not yet implemented (provisioning deferred). At provisioning,
// implement it as bucket.Object(key).Delete(ctx), treating storage.ErrObjectNotExist
// as success (idempotent, per the FileStore.Delete contract). The GDPR erasure
// path deletes blobs best-effort, so this returning ErrNotConfigured surfaces a
// misconfiguration loudly without faking success.
func (s *Store) Delete(context.Context, string) error { return ErrNotConfigured }

var _ ports.FileStore = (*Store)(nil)
