package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/angrosist/demo/internal/domain"
)

// TestCompanyUpsert_DedupAndVerification exercises the enriched Upsert against a
// real Postgres: it dedups on (country, reg_no), persists roles[]/caen/vat_status,
// writes a company_verifications row, and re-upserts in place (no duplicate row,
// updated columns, a second verification record).
func TestCompanyUpsert_DedupAndVerification(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	repo := NewCompanyRepo()

	regNo := "98765432" // unique-ish per run domain; cleaned up below
	cleanupCompany(t, "RO", regNo)

	raw := json.RawMessage(`{"name":"ACME DISTRIBUTIE SRL","caenCode":"4690"}`)
	c := &domain.Company{
		CUI:             regNo,
		Country:         "RO",
		RegNo:           regNo,
		Name:            "Acme Distributie Srl",
		Address:         "Str. Test, Nr. 1",
		County:          "Cluj",
		IsActive:        true,
		VATStatus:       "active",
		CAEN:            "4690",
		Roles:           []string{"buyer", "distributor", "wholesaler"},
		Administrators:  []string{"Ion Popescu"},
		RawData:         raw,
		RawVerification: raw,
	}

	if err := repo.Upsert(ctx, c); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if c.ID == "" {
		t.Fatal("Upsert should set company ID")
	}
	firstID := c.ID

	got, err := repo.GetByRegNo(ctx, "RO", regNo)
	if err != nil {
		t.Fatalf("GetByRegNo: %v", err)
	}
	if got.CAEN != "4690" || got.VATStatus != "active" {
		t.Errorf("caen/vat = %q/%q; want 4690/active", got.CAEN, got.VATStatus)
	}
	if len(got.Roles) != 3 {
		t.Errorf("roles = %v; want 3 elements", got.Roles)
	}

	if n := countVerifications(t, firstID); n != 1 {
		t.Fatalf("expected 1 verification row, got %d", n)
	}

	// Re-upsert with changed data: must update in place (same id, new county) and
	// add a second verification record.
	c.County = "București"
	c.Roles = []string{"buyer", "wholesaler"}
	if err := repo.Upsert(ctx, c); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if c.ID != firstID {
		t.Fatalf("re-upsert created a new row: %s != %s", c.ID, firstID)
	}

	got, err = repo.GetByRegNo(ctx, "RO", regNo)
	if err != nil {
		t.Fatalf("GetByRegNo after re-upsert: %v", err)
	}
	if got.County != "București" {
		t.Errorf("county = %q; want București (updated in place)", got.County)
	}
	if n := countCompanies(t, "RO", regNo); n != 1 {
		t.Fatalf("dedup failed: %d company rows for (RO,%s)", n, regNo)
	}
	if n := countVerifications(t, firstID); n != 2 {
		t.Fatalf("expected 2 verification rows after re-upsert, got %d", n)
	}

	// GetByCUI back-compat still resolves the same record.
	byCUI, err := repo.GetByCUI(ctx, regNo)
	if err != nil {
		t.Fatalf("GetByCUI: %v", err)
	}
	if byCUI.ID != firstID {
		t.Errorf("GetByCUI id = %s; want %s", byCUI.ID, firstID)
	}
}

// TestCompanyUpsert_MinimalNoVerification covers the save_lead recovery path:
// a minimal record (no VAT/admins/raw) upserts the company without writing a
// company_verifications row.
func TestCompanyUpsert_MinimalNoVerification(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	repo := NewCompanyRepo()

	regNo := "55554444"
	cleanupCompany(t, "RO", regNo)

	c := &domain.Company{CUI: regNo, Country: "RO", RegNo: regNo, Name: "Minimal Srl", IsActive: true}
	if err := repo.Upsert(ctx, c); err != nil {
		t.Fatalf("upsert minimal: %v", err)
	}
	if n := countVerifications(t, c.ID); n != 0 {
		t.Fatalf("minimal upsert should write no verification row, got %d", n)
	}
}

func cleanupCompany(t *testing.T, country, regNo string) {
	t.Helper()
	_, err := GetPool().Exec(context.Background(),
		`DELETE FROM companies WHERE country = $1 AND reg_no = $2`, country, regNo)
	if err != nil {
		t.Fatalf("cleanup company: %v", err)
	}
}

func countCompanies(t *testing.T, country, regNo string) int {
	t.Helper()
	var n int
	if err := GetPool().QueryRow(context.Background(),
		`SELECT count(*) FROM companies WHERE country = $1 AND reg_no = $2`,
		country, regNo).Scan(&n); err != nil {
		t.Fatalf("count companies: %v", err)
	}
	return n
}

func countVerifications(t *testing.T, companyID string) int {
	t.Helper()
	var n int
	if err := GetPool().QueryRow(context.Background(),
		`SELECT count(*) FROM company_verifications WHERE company_id = $1`,
		companyID).Scan(&n); err != nil {
		t.Fatalf("count verifications: %v", err)
	}
	return n
}
