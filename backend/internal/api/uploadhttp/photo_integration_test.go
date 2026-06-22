package uploadhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/angrosist/demo/internal/domain"
	pgadapter "github.com/angrosist/demo/internal/persistence/postgres"
	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/storage/localfs"
)

// requireDB skips the test unless DATABASE_URL points at a reachable Postgres and
// ensures the schema is present (the scratch cluster is migrated once per run).
func requireDB(t *testing.T) {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := pgadapter.GetPool().Ping(ctx); err != nil {
		t.Skipf("Postgres not reachable: %v", err)
	}
	applyMigrations(t)
}

var migrateOnce bool

// applyMigrations runs every migrations/*.sql in lexical order against the scratch
// DB. Migrations are forward-only/additive and use IF NOT EXISTS, so re-running is
// safe.
func applyMigrations(t *testing.T) {
	t.Helper()
	if migrateOnce {
		return
	}
	dir := repoMigrationsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	ctx := context.Background()
	for _, f := range files {
		sql, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := pgadapter.GetPool().Exec(ctx, string(sql)); err != nil {
			// Tolerate re-runs against a persistent scratch cluster: not every
			// migration uses IF NOT EXISTS, so duplicate-object errors (the schema is
			// already present) are benign here.
			if isAlreadyExists(err) {
				continue
			}
			t.Fatalf("apply migration %s: %v", f, err)
		}
	}
	migrateOnce = true
}

// isAlreadyExists reports whether err is a Postgres "object already exists" error
// (duplicate table/index/constraint), used to make migration re-runs idempotent
// for migrations that omit IF NOT EXISTS.
func isAlreadyExists(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42P07", // duplicate_table / index
			"42710", // duplicate_object
			"42711", // duplicate_column (defensive)
			"42701": // duplicate_column
			return true
		}
	}
	return strings.Contains(err.Error(), "already exists")
}

// repoMigrationsDir walks up from the test working dir to find backend/migrations.
func repoMigrationsDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		cand := filepath.Join(dir, "migrations")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate migrations dir")
	return ""
}

// dbGuard adapts the real ConversationRepo to the SellerConversationGuard seam for
// the integration test (mirrors the production app.sellerConversationGuard).
type dbGuard struct{ conv ports.ConversationRepo }

func (g dbGuard) IsSellerConversation(ctx context.Context, id string) (bool, error) {
	c, err := g.conv.GetByID(ctx, id)
	if err != nil {
		return false, ports.ErrNotFound
	}
	return c.Vertical == domain.VerticalPalletClearance && c.Intent == domain.IntentSell, nil
}

// newSellerConversation inserts a palletclearance/sell conversation and returns id.
func newSellerConversation(t *testing.T, vertical, intent string) string {
	t.Helper()
	conv, err := pgadapter.NewConversationRepo().CreateWith(context.Background(), "web", vertical, intent)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return conv.ID
}

func newIntegrationPhotoService(t *testing.T) *PhotoService {
	t.Helper()
	store := localfs.New(t.TempDir())
	docs := pgadapter.NewDocumentRepo()
	guard := dbGuard{conv: pgadapter.NewConversationRepo()}
	return NewPhotoService(store, docs, guard, 0, 3)
}

// TestPhotoUpload_Integration_SellerAcceptsImage drives the public endpoint against
// real Postgres + localfs: a seller conversation accepts a png and a documents row
// (owner=conversation/kind=photo) is created.
func TestPhotoUpload_Integration_SellerAcceptsImage(t *testing.T) {
	requireDB(t)
	svc := newIntegrationPhotoService(t)
	convID := newSellerConversation(t, domain.VerticalPalletClearance, domain.IntentSell)

	body, ct := buildMultipart(t, nil, "lot.png", pngBytes())
	rec := doPhotoUpload(t, svc, convID, body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	n, err := pgadapter.NewDocumentRepo().CountByOwnerKind(context.Background(), "conversation", convID, "photo")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("documents for conversation = %d; want 1", n)
	}
}

// TestPhotoUpload_Integration_NonSellerRejected asserts a buyer conversation cannot
// receive public photo uploads.
func TestPhotoUpload_Integration_NonSellerRejected(t *testing.T) {
	requireDB(t)
	svc := newIntegrationPhotoService(t)
	convID := newSellerConversation(t, domain.VerticalPalletClearance, domain.IntentBuy)

	body, ct := buildMultipart(t, nil, "lot.png", pngBytes())
	rec := doPhotoUpload(t, svc, convID, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for non-seller conversation", rec.Code)
	}
}

// TestPhotoUpload_Integration_NonImageRejected asserts a PDF is rejected.
func TestPhotoUpload_Integration_NonImageRejected(t *testing.T) {
	requireDB(t)
	svc := newIntegrationPhotoService(t)
	convID := newSellerConversation(t, domain.VerticalPalletClearance, domain.IntentSell)

	body, ct := buildMultipart(t, nil, "lot.png", pdfBytes())
	rec := doPhotoUpload(t, svc, convID, body, ct)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for non-image", rec.Code)
	}
}

// TestPhotoUpload_Integration_Oversize asserts the size cap trips.
func TestPhotoUpload_Integration_Oversize(t *testing.T) {
	requireDB(t)
	store := localfs.New(t.TempDir())
	guard := dbGuard{conv: pgadapter.NewConversationRepo()}
	svc := NewPhotoService(store, pgadapter.NewDocumentRepo(), guard, 64, 3)
	convID := newSellerConversation(t, domain.VerticalPalletClearance, domain.IntentSell)

	big := append(pngBytes(), bytes.Repeat([]byte("A"), 4096)...)
	body, ct := buildMultipart(t, nil, "big.png", big)
	rec := doPhotoUpload(t, svc, convID, body, ct)
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 413/400 for oversize", rec.Code)
	}
}

// TestPhotoUpload_Integration_PerConversationCap asserts the cap returns 409.
func TestPhotoUpload_Integration_PerConversationCap(t *testing.T) {
	requireDB(t)
	store := localfs.New(t.TempDir())
	guard := dbGuard{conv: pgadapter.NewConversationRepo()}
	svc := NewPhotoService(store, pgadapter.NewDocumentRepo(), guard, 0, 2)
	convID := newSellerConversation(t, domain.VerticalPalletClearance, domain.IntentSell)

	for i := 0; i < 2; i++ {
		body, ct := buildMultipart(t, nil, "lot.png", pngBytes())
		if rec := doPhotoUpload(t, svc, convID, body, ct); rec.Code != http.StatusOK {
			t.Fatalf("upload %d status = %d; want 200; body=%s", i, rec.Code, rec.Body.String())
		}
	}
	// Third exceeds the cap.
	body, ct := buildMultipart(t, nil, "lot.png", pngBytes())
	rec := doPhotoUpload(t, svc, convID, body, ct)
	if rec.Code != http.StatusConflict && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 409/400 over the per-conversation cap", rec.Code)
	}
}

// TestDocumentRepo_Integration_Reassign asserts CountByOwnerKind + Reassign move
// photos from a conversation to a listing (the seller-submit gate's durable step).
func TestDocumentRepo_Integration_Reassign(t *testing.T) {
	requireDB(t)
	repo := pgadapter.NewDocumentRepo()
	ctx := context.Background()
	convID := newSellerConversation(t, domain.VerticalPalletClearance, domain.IntentSell)
	listingID := newSellerConversation(t, domain.VerticalPalletClearance, domain.IntentSell) // any UUID owner

	for i := 0; i < 2; i++ {
		if err := repo.Create(ctx, &domain.Document{
			OwnerType: "conversation", OwnerID: convID, Kind: "photo",
			GCSKey: "conversation/" + convID + "/k", Mime: "image/png", SizeBytes: 10,
		}); err != nil {
			t.Fatalf("create document: %v", err)
		}
	}

	before, _ := repo.CountByOwnerKind(ctx, "conversation", convID, "photo")
	if before != 2 {
		t.Fatalf("conversation photos before = %d; want 2", before)
	}

	moved, err := repo.Reassign(ctx, "conversation", convID, "listing", listingID, "photo")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if moved != 2 {
		t.Fatalf("reassigned = %d; want 2", moved)
	}

	afterConv, _ := repo.CountByOwnerKind(ctx, "conversation", convID, "photo")
	afterListing, _ := repo.CountByOwnerKind(ctx, "listing", listingID, "photo")
	if afterConv != 0 || afterListing != 2 {
		t.Fatalf("after reassign conv=%d listing=%d; want 0/2", afterConv, afterListing)
	}
}
