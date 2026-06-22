// Package localfs is the local-filesystem adapter for the ports.FileStore seam.
// It runs in docker compose and local dev: it writes uploaded blobs under a root
// directory and serves them back through the GET /uploads/{key} route (see
// FileServer). It is the cloud-ready stand-in for the GCS adapter, which lands at
// provisioning (see internal/storage/gcs). No vendor SDK is imported here.
package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// DefaultRoot is used when FILESTORE_DIR is unset. It is relative to the process
// working directory so the demo works out of the box; prod uses GCS instead.
const DefaultRoot = "./_uploads"

// urlPrefix is the route under which stored objects are served back in dev. It is
// kept in sync with the GET handler registered in cmd/server.
const urlPrefix = "/uploads/"

// ErrUnsafeKey is returned when a storage key would escape the root directory
// (path traversal) or is otherwise malformed.
var ErrUnsafeKey = errors.New("localfs: unsafe storage key")

// Store is the local-filesystem FileStore. It is safe for concurrent use.
type Store struct {
	root string
}

// New constructs a Store rooted at dir. When dir is empty it falls back to
// DefaultRoot. The directory is created lazily on first Put.
func New(dir string) *Store {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = DefaultRoot
	}
	return &Store{root: dir}
}

// Root returns the configured root directory (used by the file-serving route).
func (s *Store) Root() string { return s.root }

// SanitizeKey normalizes a storage key into a safe, forward-slash relative path
// and rejects any attempt at path traversal (leading "/", "..", absolute paths,
// or NUL bytes). It is exported so the file-serving route can apply the identical
// check before resolving a path under the root.
func SanitizeKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrUnsafeKey
	}
	if strings.ContainsRune(key, 0) {
		return "", ErrUnsafeKey
	}
	// Reject Windows-style backslashes outright rather than risk interpretation.
	if strings.ContainsRune(key, '\\') {
		return "", ErrUnsafeKey
	}
	// Reject absolute keys: storage keys are always relative to the root.
	if strings.HasPrefix(key, "/") {
		return "", ErrUnsafeKey
	}
	// Reject any traversal attempt up front (before normalization) so a key that
	// would resolve outside — or merely tries to — is refused, not silently
	// rewritten. The server generates keys, so legitimate keys never contain "..".
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", ErrUnsafeKey
		}
	}
	// Normalize "." segments and double slashes.
	clean := path.Clean(key)
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrUnsafeKey
	}
	return clean, nil
}

// resolve maps a sanitized key to an absolute path guaranteed to live under root.
func (s *Store) resolve(key string) (string, error) {
	clean, err := SanitizeKey(key)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(s.root)
	if err != nil {
		return "", fmt.Errorf("localfs: resolve root: %w", err)
	}
	full := filepath.Join(absRoot, filepath.FromSlash(clean))
	// Defense in depth: ensure the joined path is still under the root.
	if full != absRoot && !strings.HasPrefix(full, absRoot+string(os.PathSeparator)) {
		return "", ErrUnsafeKey
	}
	return full, nil
}

// Put streams r to the file at key under the root, creating parent directories as
// needed. contentType is accepted for port symmetry but not persisted (the
// document row records the mime). A traversal-unsafe key is rejected.
func (s *Store) Put(ctx context.Context, key string, contentType string, r io.Reader) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("localfs: mkdir: %w", err)
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("localfs: open %q: %w", key, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		// Best-effort cleanup of a partially written file.
		_ = f.Close()
		_ = os.Remove(full)
		return fmt.Errorf("localfs: write %q: %w", key, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("localfs: sync %q: %w", key, err)
	}
	return nil
}

// URL returns the dev-served path for key (GET /uploads/{key}). The key is
// sanitized; an unsafe key yields an empty string so callers never emit a bad URL.
func (s *Store) URL(key string) string {
	clean, err := SanitizeKey(key)
	if err != nil {
		return ""
	}
	return urlPrefix + clean
}

// SignedURL returns the same served path as URL in local dev (ttl is ignored).
// The GCS adapter returns a genuine time-limited signed URL instead.
func (s *Store) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	u := s.URL(key)
	if u == "" {
		return "", ErrUnsafeKey
	}
	return u, nil
}

// Delete removes the file at key under the root. It is idempotent: a missing file
// (or a key that resolves to nothing) is not an error, so a retried GDPR erasure
// never fails on an already-deleted blob. A traversal-unsafe key is rejected.
func (s *Store) Delete(_ context.Context, key string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("localfs: delete %q: %w", key, err)
	}
	return nil
}

// Open returns a reader for a stored object, used by the file-serving route. The
// caller is responsible for closing it.
func (s *Store) Open(key string) (io.ReadCloser, error) {
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	return f, nil
}
