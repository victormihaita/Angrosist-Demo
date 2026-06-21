package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/angrosist/demo/internal/domain"
)

// insertCompany inserts a minimal company row and returns its id, so the listing
// / buyer-profile FKs (company_id) are satisfiable.
func insertCompany(t *testing.T, cui string) string {
	t.Helper()
	var id string
	err := GetPool().QueryRow(context.Background(), `
		INSERT INTO companies (country, reg_no, cui, name, is_active)
		VALUES ('RO', $1, $1, 'TEST SRL', true)
		ON CONFLICT (country, reg_no) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`, cui).Scan(&id)
	if err != nil {
		t.Fatalf("insert company: %v", err)
	}
	return id
}

// insertLead inserts a thin lead for a conversation with the given vertical/intent
// and returns its id.
func insertLead(t *testing.T, convID, companyID, vertical, intent string) string {
	t.Helper()
	var id string
	err := GetPool().QueryRow(context.Background(), `
		INSERT INTO leads (conversation_id, company_id, status, vertical, intent)
		VALUES ($1::uuid, $2::uuid, 'new', $3, $4) RETURNING id::text
	`, convID, companyID, vertical, intent).Scan(&id)
	if err != nil {
		t.Fatalf("insert lead: %v", err)
	}
	return id
}

// TestConversationVerticalIntentRoundTrip asserts CreateWith persists the
// vertical/intent (migration 026) and GetByID reads them back; the no-arg Create
// defaults to angrosist/buy.
func TestConversationVerticalIntentRoundTrip(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	repo := NewConversationRepo()

	// Tagged create.
	conv, err := repo.CreateWith(ctx, "web", domain.VerticalPalletClearance, domain.IntentSell)
	if err != nil {
		t.Fatalf("CreateWith: %v", err)
	}
	if conv.Vertical != domain.VerticalPalletClearance || conv.Intent != domain.IntentSell {
		t.Fatalf("create returned vertical/intent %q/%q", conv.Vertical, conv.Intent)
	}
	got, err := repo.GetByID(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Vertical != domain.VerticalPalletClearance || got.Intent != domain.IntentSell {
		t.Fatalf("reloaded vertical/intent %q/%q", got.Vertical, got.Intent)
	}

	// Legacy create defaults.
	def, err := repo.Create(ctx, "web")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if def.Vertical != domain.VerticalAngrosist || def.Intent != domain.IntentBuy {
		t.Fatalf("default create vertical/intent %q/%q", def.Vertical, def.Intent)
	}
}

// TestListingRepo_CreateAndUpsert asserts the seller typed-request writer inserts
// then upserts-in-place a listings row (migration 019).
func TestListingRepo_CreateAndUpsert(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	repo := NewListingRepo()

	convID := newConversation(t)
	companyID := insertCompany(t, "100000001")
	leadID := insertLead(t, convID, companyID, domain.VerticalPalletClearance, domain.IntentSell)

	qty := 12.5
	price := 999.0
	expiry := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	l := &domain.Listing{
		LeadID:       leadID,
		CompanyID:    companyID,
		StockType:    "overstock",
		Quantity:     &qty,
		Location:     "Cluj",
		Country:      "RO",
		Expiry:       &expiry,
		TargetPrice:  &price,
		Confidential: true,
		Status:       "active",
	}
	if err := repo.Create(ctx, l); err != nil {
		t.Fatalf("Create listing: %v", err)
	}
	if l.ID == "" {
		t.Fatal("expected listing id assigned")
	}

	// Upsert by lead updates in place (one row per lead).
	l2 := &domain.Listing{LeadID: leadID, CompanyID: companyID, StockType: "returns",
		Country: "BG", Confidential: false}
	if err := repo.UpsertByLead(ctx, l2); err != nil {
		t.Fatalf("UpsertByLead: %v", err)
	}

	var count int
	var stockType, country string
	var confidential bool
	if err := GetPool().QueryRow(ctx,
		`SELECT count(*) FROM listings WHERE lead_id = $1::uuid`, leadID).Scan(&count); err != nil {
		t.Fatalf("count listings: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 listing per lead, got %d", count)
	}
	if err := GetPool().QueryRow(ctx,
		`SELECT stock_type, country, confidential FROM listings WHERE lead_id = $1::uuid`, leadID,
	).Scan(&stockType, &country, &confidential); err != nil {
		t.Fatalf("read listing: %v", err)
	}
	if stockType != "returns" || country != "BG" || confidential {
		t.Fatalf("upsert did not update in place: stock=%q country=%q confidential=%v", stockType, country, confidential)
	}
}

// TestBuyerProfileRepo_Upsert asserts the buyer typed-request writer inserts then
// updates-in-place a buyer_profiles row keyed by (company_id, vertical)
// (migration 020).
func TestBuyerProfileRepo_Upsert(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	repo := NewBuyerProfileRepo()

	companyID := insertCompany(t, "100000002")

	p := &domain.BuyerProfile{
		CompanyID:    companyID,
		Vertical:     domain.VerticalPalletClearance,
		Categories:   []string{},
		Countries:    []string{"RO", "BG"},
		NearExpiryOK: true,
		Subscribed:   true,
	}
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert insert: %v", err)
	}
	firstID := p.ID
	if firstID == "" {
		t.Fatal("expected buyer profile id assigned")
	}

	// Second upsert for the same (company, vertical) updates in place.
	p2 := &domain.BuyerProfile{
		CompanyID:    companyID,
		Vertical:     domain.VerticalPalletClearance,
		Categories:   []string{},
		Countries:    []string{"RO"},
		NearExpiryOK: false,
		Subscribed:   false,
	}
	if err := repo.Upsert(ctx, p2); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	if p2.ID != firstID {
		t.Fatalf("expected upsert to reuse the same row id %q, got %q", firstID, p2.ID)
	}

	var count int
	var nearExpiry, subscribed bool
	if err := GetPool().QueryRow(ctx,
		`SELECT count(*) FROM buyer_profiles WHERE company_id = $1::uuid AND vertical = $2`,
		companyID, domain.VerticalPalletClearance).Scan(&count); err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 buyer profile per (company,vertical), got %d", count)
	}
	if err := GetPool().QueryRow(ctx,
		`SELECT near_expiry_ok, subscribed FROM buyer_profiles WHERE id = $1::uuid`, firstID,
	).Scan(&nearExpiry, &subscribed); err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if nearExpiry || subscribed {
		t.Fatalf("upsert did not update flags: near_expiry=%v subscribed=%v", nearExpiry, subscribed)
	}
}
