package postgres

import (
	"context"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
)

// ListingRepo is the Postgres adapter for the PalletClearance seller typed
// request (migration 019). One listing per lead (UNIQUE lead_id). All SQL is
// parameterized.
type ListingRepo struct{}

// NewListingRepo constructs the listings adapter.
func NewListingRepo() *ListingRepo { return &ListingRepo{} }

// defaultListingStatus mirrors the column default in migration 019.
const defaultListingStatus = "active"

func (r *ListingRepo) Create(ctx context.Context, l *domain.Listing) error {
	status := l.Status
	if status == "" {
		status = defaultListingStatus
	}
	row := GetPool().QueryRow(ctx, `
		INSERT INTO listings (lead_id, company_id, category_id, stock_type, quantity,
		                      location, country, expiry, target_price, confidential, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`,
		l.LeadID, nullStr(l.CompanyID), nullStr(l.CategoryID), nullStr(l.StockType),
		l.Quantity, nullStr(l.Location), nullStr(l.Country), l.Expiry, l.TargetPrice,
		l.Confidential, status,
	)
	if err := row.Scan(&l.ID, &l.CreatedAt); err != nil {
		return fmt.Errorf("create listing: %w", err)
	}
	return nil
}

// UpsertByLead inserts the listing or, when one already exists for the lead,
// updates it in place (idempotent submit path). It relies on the UNIQUE lead_id
// constraint from migration 019.
func (r *ListingRepo) UpsertByLead(ctx context.Context, l *domain.Listing) error {
	status := l.Status
	if status == "" {
		status = defaultListingStatus
	}
	row := GetPool().QueryRow(ctx, `
		INSERT INTO listings (lead_id, company_id, category_id, stock_type, quantity,
		                      location, country, expiry, target_price, confidential, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (lead_id) DO UPDATE SET
			company_id   = EXCLUDED.company_id,
			category_id  = EXCLUDED.category_id,
			stock_type   = EXCLUDED.stock_type,
			quantity     = EXCLUDED.quantity,
			location     = EXCLUDED.location,
			country      = EXCLUDED.country,
			expiry       = EXCLUDED.expiry,
			target_price = EXCLUDED.target_price,
			confidential = EXCLUDED.confidential,
			status       = EXCLUDED.status,
			updated_at   = now()
		RETURNING id, created_at
	`,
		l.LeadID, nullStr(l.CompanyID), nullStr(l.CategoryID), nullStr(l.StockType),
		l.Quantity, nullStr(l.Location), nullStr(l.Country), l.Expiry, l.TargetPrice,
		l.Confidential, status,
	)
	if err := row.Scan(&l.ID, &l.CreatedAt); err != nil {
		return fmt.Errorf("upsert listing: %w", err)
	}
	return nil
}
