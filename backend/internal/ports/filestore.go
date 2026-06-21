package ports

import (
	"context"
	"io"
	"time"
)

// FileStore is the single seam to binary object storage (seller photos, buyer
// product lists, P2 offers). The bytes always live behind this port — never on
// instance disk in prod (DATA_MODEL invariant #6) — and the inner layers depend
// only on this interface, so swapping the local-filesystem adapter for the GCS
// adapter is a wiring change, not a domain change.
//
// Two adapters satisfy it:
//
//   - internal/storage/localfs — runs in docker compose / locally; writes under a
//     root directory and serves bytes via a GET /uploads/{key} route. URL returns
//     that served path.
//   - internal/storage/gcs (DEFERRED to provisioning) — streams to a private GCS
//     bucket and returns time-limited *signed* download URLs from SignedURL.
//     Not implemented yet; the stub returns a clear "not configured" error so a
//     misconfiguration fails loudly instead of faking success.
type FileStore interface {
	// Put streams r to durable storage under key with the given content type. key
	// is the caller-namespaced, already-sanitized storage key (the handler builds
	// it as <owner_type>/<owner_id>/<uuid>-<sanitized-filename>). Implementations
	// must additionally guard against path traversal before touching disk.
	Put(ctx context.Context, key string, contentType string, r io.Reader) error

	// URL returns a directly usable URL/path for key. The local adapter returns
	// the path served by the GET /uploads/{key} route (good for dev/docker). The
	// GCS adapter will return a long-lived public URL only for public buckets;
	// for the private prod bucket, prefer SignedURL.
	URL(key string) string

	// SignedURL returns a time-limited download URL for key. This is how the
	// dashboard/portal will fetch private objects in prod (GCS signed GET URLs).
	// The local adapter ignores ttl and returns the same path as URL (dev only).
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}
