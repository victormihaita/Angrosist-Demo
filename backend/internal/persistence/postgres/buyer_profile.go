package postgres

import (
	"context"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
)

// BuyerProfileRepo is the Postgres adapter for the PalletClearance buyer
// standing-demand profile (migration 020). It upserts on (company_id, vertical)
// so a re-submitted turn updates the profile rather than duplicating it. All SQL
// is parameterized; categories/countries are bound as native Postgres arrays.
type BuyerProfileRepo struct{}

// NewBuyerProfileRepo constructs the buyer_profiles adapter.
func NewBuyerProfileRepo() *BuyerProfileRepo { return &BuyerProfileRepo{} }

// Upsert inserts the buyer profile or updates the existing one for the same
// (company_id, vertical). Migration 020 has no unique constraint on that pair, so
// the upsert is expressed as a guarded UPDATE-then-INSERT in a single statement
// via a CTE — keeping it a single round-trip and parameterized.
func (r *BuyerProfileRepo) Upsert(ctx context.Context, p *domain.BuyerProfile) error {
	if p.Vertical == "" {
		p.Vertical = domain.DefaultVertical
	}
	categories := p.Categories
	if categories == nil {
		categories = []string{}
	}
	countries := p.Countries
	if countries == nil {
		countries = []string{}
	}
	row := GetPool().QueryRow(ctx, `
		WITH upd AS (
			UPDATE buyer_profiles
			SET categories = $3::uuid[], countries = $4::text[],
			    near_expiry_ok = $5, subscribed = $6, updated_at = now()
			WHERE company_id = $1 AND vertical = $2
			RETURNING id, created_at
		), ins AS (
			INSERT INTO buyer_profiles (company_id, vertical, categories, countries, near_expiry_ok, subscribed)
			SELECT $1, $2, $3::uuid[], $4::text[], $5, $6
			WHERE NOT EXISTS (SELECT 1 FROM upd)
			RETURNING id, created_at
		)
		SELECT id, created_at FROM upd
		UNION ALL
		SELECT id, created_at FROM ins
	`,
		p.CompanyID, p.Vertical, categories, countries, p.NearExpiryOK, p.Subscribed,
	)
	if err := row.Scan(&p.ID, &p.CreatedAt); err != nil {
		return fmt.Errorf("upsert buyer profile: %w", err)
	}
	return nil
}
