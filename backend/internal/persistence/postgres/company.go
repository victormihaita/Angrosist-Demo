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
				 vat_status, caen, roles, registration_number, registration_date,
				 legal_form, e_factura, raw_data, verified_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
			ON CONFLICT (country, reg_no) DO UPDATE SET
				cui                 = EXCLUDED.cui,
				name                = EXCLUDED.name,
				address             = EXCLUDED.address,
				county              = EXCLUDED.county,
				is_active           = EXCLUDED.is_active,
				vat_status          = EXCLUDED.vat_status,
				caen                = EXCLUDED.caen,
				roles               = EXCLUDED.roles,
				registration_number = EXCLUDED.registration_number,
				registration_date   = EXCLUDED.registration_date,
				legal_form          = EXCLUDED.legal_form,
				e_factura           = EXCLUDED.e_factura,
				raw_data            = EXCLUDED.raw_data,
				verified_at         = NOW(),
				updated_at          = NOW()
			RETURNING id
		`,
			cui, country, regNo, c.Name, c.Address, c.County, c.IsActive,
			nullStr(c.VATStatus), nullStr(c.CAEN), roles,
			nullStr(c.RegistrationNumber), nullTime(c.RegistrationDate),
			nullStr(c.LegalForm), c.EFactura, nullBytes(c.RawData),
		).Scan(&companyID)
		if err != nil {
			return fmt.Errorf("upsert company (%s/%s): %w", country, regNo, err)
		}
		c.ID = companyID

		if err := upsertFinancials(ctx, tx, companyID, c.Financials); err != nil {
			return fmt.Errorf("upsert financials (%s/%s): %w", country, regNo, err)
		}

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

// upsertFinancials idempotently writes one company_financials row per year from
// the company's provider-supplied snapshots. It is keyed on (company_id, year)
// (UNIQUE, migration 014): an existing year is updated in place, never duplicated.
// An empty slice is a no-op so the save_lead recovery path writes nothing.
func upsertFinancials(ctx context.Context, tx pgx.Tx, companyID string, years []domain.CompanyFinancialYear) error {
	for _, y := range years {
		if y.Year <= 0 {
			continue
		}
		raw, err := json.Marshal(y)
		if err != nil {
			return fmt.Errorf("marshal financial %d: %w", y.Year, err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO company_financials
				(company_id, year, turnover, net_profit, employees, raw)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (company_id, year) DO UPDATE SET
				turnover   = EXCLUDED.turnover,
				net_profit = EXCLUDED.net_profit,
				employees  = EXCLUDED.employees,
				raw        = EXCLUDED.raw
		`, companyID, y.Year, y.Turnover, y.NetProfit, y.Employees, raw)
		if err != nil {
			return fmt.Errorf("upsert financial year %d: %w", y.Year, err)
		}
	}
	return nil
}

// nullTime renders a nil *time.Time as SQL NULL and a non-nil one as its value,
// so a missing incorporation date is stored as NULL rather than the zero time.
func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
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
			COALESCE(c.registration_number, ''),
			c.registration_date,
			COALESCE(c.legal_form, ''),
			COALESCE(c.e_factura, false),
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
		&d.RegistrationNumber, &d.RegistrationDate, &d.LegalForm, &d.EFactura,
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
			d.Administrators = decodeAdminNames(vAdmins)
		}
		if vChecked != nil {
			ver.CheckedAt = *vChecked
		}
		d.Verification = ver
	}
	// Ensure vat_status is populated on the response: prefer the company column,
	// fall back to the latest verification's value when the column is empty.
	if d.VATStatus == "" && vVAT != nil {
		d.VATStatus = *vVAT
	}

	fin, err := r.financials(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	d.Financials = fin
	return &d, nil
}

// financials returns the company's financial snapshots (newest year first), or
// an empty slice when none exist (company_financials, migrations 014 + 027). The
// per-year CAEN description is recovered from the stored raw payload when present.
func (r *CompanyRepo) financials(ctx context.Context, companyID string) ([]domain.CompanyFinancialView, error) {
	rows, err := GetPool().Query(ctx, `
		SELECT year, turnover, net_profit, employees, raw
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
		var raw []byte
		if err := rows.Scan(&v.Year, &v.Turnover, &v.NetProfit, &v.Employees, &raw); err != nil {
			return nil, fmt.Errorf("scan financial: %w", err)
		}
		if len(raw) > 0 {
			var probe struct {
				CAENDescription string `json:"CAENDescription"`
			}
			if err := json.Unmarshal(raw, &probe); err == nil {
				v.CAENDescription = probe.CAENDescription
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// decodeAdminNames decodes the company_verifications.administrators JSONB (a JSON
// array of names, as written by insertVerification) into a slice. It returns nil
// on any decode error so a malformed payload never fails the detail query.
func decodeAdminNames(b []byte) []string {
	var names []string
	if err := json.Unmarshal(b, &names); err != nil {
		return nil
	}
	return names
}

var _ ports.CompanyRepo = (*CompanyRepo)(nil)
