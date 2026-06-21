package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
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

// ListPage returns one keyset page of directory companies matching the filter.
// The role filter uses array containment (companies.roles @> ARRAY[role]) which
// is served by the GIN index companies_roles_gin (migration 013); name search is
// ILIKE. Ordering is created_at DESC, id DESC; it fetches Limit+1 for has-next.
func (r *CompanyRepo) ListPage(ctx context.Context, f domain.CompanyFilter) ([]*domain.CompanySummary, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}

	if f.Role != "" {
		add("roles @> ARRAY[$%d]::text[]", f.Role)
	}
	if f.Country != "" {
		add("country = $%d", f.Country)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add("name ILIKE $%d", "%"+q+"%")
	}
	if f.After != nil {
		args = append(args, f.After.CreatedAt)
		ca := len(args)
		args = append(args, f.After.ID)
		id := len(args)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d::uuid)", ca, id))
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	args = append(args, limit+1)

	query := `
		SELECT
			id::text,
			COALESCE(name, ''),
			COALESCE(cui, ''),
			COALESCE(country, 'RO'),
			COALESCE(reg_no, ''),
			COALESCE(caen, ''),
			COALESCE(vat_status, ''),
			roles,
			created_at
		FROM companies
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY created_at DESC, id DESC
		LIMIT $` + fmt.Sprint(len(args))

	rows, err := GetPool().Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list companies page: %w", err)
	}
	defer rows.Close()

	var out []*domain.CompanySummary
	for rows.Next() {
		var s domain.CompanySummary
		if err := rows.Scan(
			&s.ID, &s.Name, &s.CUI, &s.Country, &s.RegNo, &s.CAEN, &s.VATStatus,
			&s.Roles, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan company summary: %w", err)
		}
		if s.Roles == nil {
			s.Roles = []string{}
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// Detail returns the directory detail for a company id: roles[], the latest
// verification, and any financial-year snapshots. It returns ports.ErrNotFound
// when no such company exists.
func (r *CompanyRepo) Detail(ctx context.Context, id string) (*domain.CompanyDetail, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT
			c.id::text,
			COALESCE(c.name, ''),
			COALESCE(c.cui, ''),
			COALESCE(c.country, 'RO'),
			COALESCE(c.reg_no, ''),
			COALESCE(c.caen, ''),
			COALESCE(c.vat_status, ''),
			c.roles,
			c.created_at,
			COALESCE(c.address, ''),
			COALESCE(c.county, ''),
			c.is_active,
			cv.source, cv.vat_status, cv.administrators, cv.checked_at
		FROM companies c
		LEFT JOIN LATERAL (
			SELECT source, vat_status, administrators, checked_at
			FROM company_verifications
			WHERE company_id = c.id
			ORDER BY checked_at DESC LIMIT 1
		) cv ON true
		WHERE c.id = $1::uuid
	`, id)

	var d domain.CompanyDetail
	var vSource, vVAT *string
	var vAdmins []byte
	var vChecked *time.Time
	if err := row.Scan(
		&d.ID, &d.Name, &d.CUI, &d.Country, &d.RegNo, &d.CAEN, &d.VATStatus,
		&d.Roles, &d.CreatedAt, &d.Address, &d.County, &d.IsActive,
		&vSource, &vVAT, &vAdmins, &vChecked,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("company detail: %w", err)
	}
	if d.Roles == nil {
		d.Roles = []string{}
	}
	if vSource != nil {
		ver := &domain.CompanyVerificationView{Source: *vSource}
		if vVAT != nil {
			ver.VATStatus = *vVAT
		}
		if len(vAdmins) > 0 {
			ver.Administrators = append([]byte(nil), vAdmins...)
		}
		if vChecked != nil {
			ver.CheckedAt = *vChecked
		}
		d.Verification = ver
	}

	fin, err := r.financials(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	d.Financials = fin
	return &d, nil
}

// financials returns the company's turnover snapshots (newest year first), or an
// empty slice when none exist (company_financials, migration 014).
func (r *CompanyRepo) financials(ctx context.Context, companyID string) ([]domain.CompanyFinancialView, error) {
	rows, err := GetPool().Query(ctx, `
		SELECT year, turnover
		FROM company_financials
		WHERE company_id = $1::uuid
		ORDER BY year DESC
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("company financials: %w", err)
	}
	defer rows.Close()

	var out []domain.CompanyFinancialView
	for rows.Next() {
		var v domain.CompanyFinancialView
		if err := rows.Scan(&v.Year, &v.Turnover); err != nil {
			return nil, fmt.Errorf("scan financial: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

var _ ports.CompanyRepo = (*CompanyRepo)(nil)
