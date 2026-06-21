package postgres

import (
	"context"
	"testing"

	"github.com/angrosist/demo/internal/domain"
)

// TestDocumentRepo_CreateAndListByOwner exercises the document index against a
// real Postgres: Create assigns id/created_at and persists the columns; the row
// is then returned by ListByOwner and scoped to its owner.
func TestDocumentRepo_CreateAndListByOwner(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	repo := NewDocumentRepo()

	// A fresh conversation gives us a valid UUID to use as a polymorphic owner_id.
	owner := newConversation(t)

	d := &domain.Document{
		OwnerType: "conversation",
		OwnerID:   owner,
		Kind:      "product_list",
		GCSKey:    "conversation/" + owner + "/uuid-list.csv",
		Mime:      "text/csv",
		SizeBytes: 1234,
	}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == "" {
		t.Fatal("Create should assign an ID")
	}
	if d.CreatedAt.IsZero() {
		t.Fatal("Create should assign CreatedAt")
	}

	got, err := repo.ListByOwner(ctx, "conversation", owner)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListByOwner returned %d rows; want 1", len(got))
	}
	g := got[0]
	if g.ID != d.ID || g.Kind != "product_list" || g.GCSKey != d.GCSKey || g.Mime != "text/csv" || g.SizeBytes != 1234 {
		t.Fatalf("round-trip mismatch: %+v", g)
	}

	// A different owner sees no documents (owner scoping).
	other := newConversation(t)
	empty, err := repo.ListByOwner(ctx, "conversation", other)
	if err != nil {
		t.Fatalf("ListByOwner(other): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no documents for other owner, got %d", len(empty))
	}

	// Cleanup so reruns stay clean.
	if _, err := GetPool().Exec(ctx, `DELETE FROM documents WHERE id = $1`, d.ID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
