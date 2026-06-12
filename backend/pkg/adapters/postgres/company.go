package postgres

import (
	"context"

	"github.com/angrosist/demo/pkg/domain"
)

type CompanyRepo struct{}

func NewCompanyRepo() *CompanyRepo { return &CompanyRepo{} }

func (r *CompanyRepo) GetByCUI(ctx context.Context, cui string) (*domain.Company, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT id, cui, name, address, county, is_active, raw_data, verified_at, created_at
		FROM companies WHERE cui = $1
	`, cui)

	var c domain.Company
	err := row.Scan(&c.ID, &c.CUI, &c.Name, &c.Address, &c.County,
		&c.IsActive, &c.RawData, &c.VerifiedAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CompanyRepo) Upsert(ctx context.Context, c *domain.Company) error {
	_, err := GetPool().Exec(ctx, `
		INSERT INTO companies (cui, name, address, county, is_active, raw_data, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (cui) DO UPDATE SET
			name        = EXCLUDED.name,
			address     = EXCLUDED.address,
			county      = EXCLUDED.county,
			is_active   = EXCLUDED.is_active,
			raw_data    = EXCLUDED.raw_data,
			verified_at = NOW()
	`, c.CUI, c.Name, c.Address, c.County, c.IsActive, nullBytes(c.RawData))
	return err
}
