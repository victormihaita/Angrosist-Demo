package postgres

import (
	"context"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// DocumentRepo is the Postgres adapter for the polymorphic document index
// (migration 021_documents). All statements are parameterized; owner_type/owner_id
// is a soft polymorphic reference (no DB FK).
type DocumentRepo struct{}

// NewDocumentRepo constructs the Postgres-backed document repository.
func NewDocumentRepo() *DocumentRepo { return &DocumentRepo{} }

// Create inserts one document row and writes the assigned id/created_at back onto
// d. mime is stored NULL when empty; size_bytes is stored NULL when zero.
func (r *DocumentRepo) Create(ctx context.Context, d *domain.Document) error {
	var size any
	if d.SizeBytes > 0 {
		size = d.SizeBytes
	}
	row := GetPool().QueryRow(ctx, `
		INSERT INTO documents (owner_type, owner_id, kind, gcs_key, mime, size_bytes)
		VALUES ($1, $2::uuid, $3, $4, $5, $6)
		RETURNING id, created_at
	`, d.OwnerType, d.OwnerID, d.Kind, d.GCSKey, nullStr(d.Mime), size)
	if err := row.Scan(&d.ID, &d.CreatedAt); err != nil {
		return fmt.Errorf("create document (owner=%s/%s kind=%s): %w", d.OwnerType, d.OwnerID, d.Kind, err)
	}
	return nil
}

// ListByOwner returns all documents for (ownerType, ownerID), newest first.
func (r *DocumentRepo) ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*domain.Document, error) {
	rows, err := GetPool().Query(ctx, `
		SELECT id, owner_type, owner_id, kind, gcs_key, COALESCE(mime, ''), COALESCE(size_bytes, 0), created_at
		FROM documents
		WHERE owner_type = $1 AND owner_id = $2::uuid
		ORDER BY created_at DESC, id DESC
	`, ownerType, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list documents (owner=%s/%s): %w", ownerType, ownerID, err)
	}
	defer rows.Close()

	var out []*domain.Document
	for rows.Next() {
		var d domain.Document
		if err := rows.Scan(&d.ID, &d.OwnerType, &d.OwnerID, &d.Kind, &d.GCSKey, &d.Mime, &d.SizeBytes, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documents: %w", err)
	}
	return out, nil
}

// CountByOwnerKind returns the number of documents bound to (ownerType, ownerID)
// with the given kind. It backs the seller-photo blocking gate.
func (r *DocumentRepo) CountByOwnerKind(ctx context.Context, ownerType, ownerID, kind string) (int, error) {
	var n int
	err := GetPool().QueryRow(ctx, `
		SELECT count(*)
		FROM documents
		WHERE owner_type = $1 AND owner_id = $2::uuid AND kind = $3
	`, ownerType, ownerID, kind).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count documents (owner=%s/%s kind=%s): %w", ownerType, ownerID, kind, err)
	}
	return n, nil
}

// Reassign re-points every document of the given kind from one owner to another
// and returns the number of rows moved. Used to attach conversation-scoped seller
// photos to the durable listing once it is created. All inputs are parameterized.
func (r *DocumentRepo) Reassign(ctx context.Context, fromOwnerType, fromOwnerID, toOwnerType, toOwnerID, kind string) (int, error) {
	tag, err := GetPool().Exec(ctx, `
		UPDATE documents
		SET owner_type = $3, owner_id = $4::uuid
		WHERE owner_type = $1 AND owner_id = $2::uuid AND kind = $5
	`, fromOwnerType, fromOwnerID, toOwnerType, toOwnerID, kind)
	if err != nil {
		return 0, fmt.Errorf("reassign documents (%s/%s -> %s/%s kind=%s): %w",
			fromOwnerType, fromOwnerID, toOwnerType, toOwnerID, kind, err)
	}
	return int(tag.RowsAffected()), nil
}

var _ ports.DocumentRepo = (*DocumentRepo)(nil)
