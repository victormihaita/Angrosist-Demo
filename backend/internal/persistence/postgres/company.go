package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
)

// CompanyRepo persists companies (the B2B directory asset) and their verification
// records. It dedups on the canonical (country, reg_no) key while keeping the
// legacy `cui` column populated for back-compat.
type CompanyRepo struct{}

func NewCompanyRepo() *CompanyRepo { return &CompanyRepo{} }

// companyColumns is the shared SELECT projection so GetByCUI / GetByRegNo scan
// identically.
const companyColumns = `id, cui, name, address, county, is_active,
	COALESCE(country, 'RO'), COALESCE(reg_no, cui),
	vat_status, caen, roles, raw_data, verified_at, created_at`

func scanCompany(row pgx.Row) (*domain.Company, error) {
	var c domain.Company
	var cui, vatStatus, caenCode *string
	if err := row.Scan(
		&c.ID, &cui, &c.Name, &c.Address, &c.County, &c.IsActive,
		&c.Country, &c.RegNo, &vatStatus, &caenCode, &c.Roles,
		&c.RawData, &c.VerifiedAt, &c.CreatedAt,
	); err != nil {
		return nil, err
	}
	if cui != nil {
		c.CUI = *cui
	}
	if vatStatus != nil {
		c.VATStatus = *vatStatus
	}
	if caenCode != nil {
		c.CAEN = *caenCode
	}
	return &c, nil
}

// GetByCUI looks up a company by the legacy CUI column (kept for save_lead's
// recovery path and existing callers).
func (r *CompanyRepo) GetByCUI(ctx context.Context, cui string) (*domain.Company, error) {
	row := GetPool().QueryRow(ctx,
		`SELECT `+companyColumns+` FROM companies WHERE cui = $1`, cui)
	return scanCompany(row)
}

// GetByRegNo looks up a company by the canonical (country, reg_no) dedup key.
func (r *CompanyRepo) GetByRegNo(ctx context.Context, country, regNo string) (*domain.Company, error) {
	row := GetPool().QueryRow(ctx,
		`SELECT `+companyColumns+` FROM companies WHERE country = $1 AND reg_no = $2`,
		country, regNo)
	return scanCompany(row)
}

// Upsert inserts or updates the company keyed on (country, reg_no), persisting
// the enriched columns and keeping `cui` populated. When the company carries
// verification data (VAT status, administrators, or a raw payload) it also writes
// a company_verifications row (source='demoanaf'). The whole operation runs in one
// transaction so the company and its verification record stay consistent.
func (r *CompanyRepo) Upsert(ctx context.Context, c *domain.Company) error {
	country := c.Country
	if country == "" {
		country = "RO"
	}
	regNo := c.RegNo
	if regNo == "" {
		regNo = c.CUI // fall back to legacy CUI for callers not yet enriched
	}
	cui := c.CUI
	if cui == "" {
		cui = regNo
	}
	roles := c.Roles
	if roles == nil {
		roles = []string{}
	}

	return pgx.BeginTxFunc(ctx, GetPool(), pgx.TxOptions{}, func(tx pgx.Tx) error {
		var companyID string
		err := tx.QueryRow(ctx, `
			INSERT INTO companies
				(cui, country, reg_no, name, address, county, is_active,
				 vat_status, caen, roles, raw_data, verified_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
			ON CONFLICT (country, reg_no) DO UPDATE SET
				cui         = EXCLUDED.cui,
				name        = EXCLUDED.name,
				address     = EXCLUDED.address,
				county      = EXCLUDED.county,
				is_active   = EXCLUDED.is_active,
				vat_status  = EXCLUDED.vat_status,
				caen        = EXCLUDED.caen,
				roles       = EXCLUDED.roles,
				raw_data    = EXCLUDED.raw_data,
				verified_at = NOW(),
				updated_at  = NOW()
			RETURNING id
		`,
			cui, country, regNo, c.Name, c.Address, c.County, c.IsActive,
			nullStr(c.VATStatus), nullStr(c.CAEN), roles, nullBytes(c.RawData),
		).Scan(&companyID)
		if err != nil {
			return fmt.Errorf("upsert company (%s/%s): %w", country, regNo, err)
		}
		c.ID = companyID

		if !hasVerificationData(c) {
			return nil
		}
		if err := insertVerification(ctx, tx, companyID, c); err != nil {
			return fmt.Errorf("record verification (%s/%s): %w", country, regNo, err)
		}
		return nil
	})
}

// hasVerificationData reports whether the company carries enough provider data to
// justify a company_verifications row. The save_lead recovery path (minimal record
// built from agent args) carries none, so it skips the write.
func hasVerificationData(c *domain.Company) bool {
	return strings.TrimSpace(c.VATStatus) != "" ||
		len(c.Administrators) > 0 ||
		len(c.RawVerification) > 0
}

// insertVerification appends an audit/cache row to company_verifications.
func insertVerification(ctx context.Context, tx pgx.Tx, companyID string, c *domain.Company) error {
	var admins any
	if len(c.Administrators) > 0 {
		b, err := json.Marshal(c.Administrators)
		if err != nil {
			return fmt.Errorf("marshal administrators: %w", err)
		}
		admins = b
	}

	var raw any
	if len(c.RawVerification) > 0 {
		raw = []byte(c.RawVerification)
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO company_verifications
			(company_id, source, vat_status, administrators, raw, checked_at)
		VALUES ($1, 'demoanaf', $2, $3, $4, NOW())
	`, companyID, nullStr(c.VATStatus), admins, raw)
	return err
}
