package domain

import "time"

// Document is a polymorphic index row for a binary blob held in object storage
// (migration 021_documents). The bytes themselves live in the FileStore (GCS in
// prod, local filesystem in dev); this row records where to find them and what
// they belong to. (OwnerType, OwnerID) is a SOFT polymorphic reference — no DB
// foreign key can span multiple owner tables — so deletion of the blob is an
// explicit app-code step on the GDPR erasure path, never an automatic cascade.
type Document struct {
	// ID is the database-assigned UUID (set on Create).
	ID string
	// OwnerType identifies the kind of entity the document belongs to, e.g.
	// "lead" | "conversation" | "listing" | "offer" | "sourcing_request".
	OwnerType string
	// OwnerID is the UUID of the owning entity (soft polymorphic FK).
	OwnerID string
	// Kind classifies the document's role, e.g. "product_list" | "photo" | "offer".
	Kind string
	// GCSKey is the durable storage key returned by the FileStore. In prod this is
	// the GCS object key; in local dev it is the namespaced filesystem-relative key.
	// Named GCSKey to match the migration column; storage-agnostic in meaning.
	GCSKey string
	// Mime is the resolved (sniffed) content type of the stored bytes.
	Mime string
	// SizeBytes is the stored object size in bytes (0 when unknown).
	SizeBytes int64
	// CreatedAt is the database-assigned creation timestamp (set on Create).
	CreatedAt time.Time
}
