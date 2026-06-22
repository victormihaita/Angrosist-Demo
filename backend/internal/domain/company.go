package domain

import (
	"encoding/json"
	"time"
)

// Company is a verified (or partially known) B2B company. It is the strategic
// directory asset described in the data model: deduped on (Country, RegNo).
//
// The demo populated only the minimal raw-ANAF fields (CUI, Name, Address,
// County, IsActive, RawData). The richer fields below are populated by the
// DemoANAF provider and persisted additively; they are zero-valued when the
// fallback raw-ANAF client or the save_lead recovery path builds the record.
type Company struct {
	ID   string
	CUI  string // legacy alias kept for back-compat; equals RegNo for RO companies
	Name string

	// Country is the ISO country of registration. Defaults to "RO"; part of the
	// (Country, RegNo) dedup key.
	Country string
	// RegNo is the canonical registration number used for dedup. For RO companies
	// this is the CUI (numeric fiscal code).
	RegNo string

	// RegistrationNumber is the ONRC trade-register J-number (e.g. "J12/1234/2020").
	// Distinct from RegNo/CUI; informational, not a dedup key.
	RegistrationNumber string

	// RegistrationDate is the company's incorporation date ("when created") from
	// ONRC. Nil when the provider does not supply it (foreign / unverified companies).
	RegistrationDate *time.Time
	// LegalForm is the legal form (e.g. "SRL", "SA", "PFA"). Empty when unknown.
	LegalForm string
	// EFactura reports whether the company is registered for e-Factura (e-invoicing).
	EFactura bool

	Address string
	County  string

	// IsActive reflects whether the company is active (not flagged inactive in ANAF).
	IsActive bool
	// VATStatus is the VAT-registration status string from the provider
	// (e.g. "active", "inactive", "not_registered", "unknown").
	VATStatus string
	// CAEN is the primary activity code (RO CAEN Rev.2). Nullable for foreign companies.
	CAEN string
	// AuthorizedCAEN holds all permitted activity codes when the provider supplies them.
	AuthorizedCAEN []string

	// Roles classifies the company for the B2B directory + Phase-2 matching. Derived
	// opportunistically from CAEN (see verification/caen). Values are the documented
	// role set: distributor/importer/wholesaler/retailer/horeca/processor/producer/
	// buyer/seller.
	Roles []string

	// Administrators are the company's administrators' names (from ONRC), when available.
	Administrators []string

	// Financials are the company's multi-year financial snapshots (turnover, net
	// profit, employees), newest year first when populated by the provider. Empty
	// when financials enrichment is disabled or the lookup failed (best-effort).
	Financials []CompanyFinancialYear

	// RawData is the full raw provider payload kept on the company row (back-compat
	// with the demo's companies.raw_data column).
	RawData []byte
	// RawVerification is the full provider payload persisted into
	// company_verifications.raw for audit/cache. For DemoANAF this is the `data`
	// object of the response envelope.
	RawVerification json.RawMessage

	VerifiedAt *time.Time
	CreatedAt  time.Time
}

// CompanyFinancialYear is a single year's financial snapshot for a company, as
// supplied by the verification provider's financials endpoint. All money/count
// fields are pointers so "not reported" (nil) is distinct from a genuine zero.
type CompanyFinancialYear struct {
	// Year is the fiscal year (e.g. 2024).
	Year int
	// Turnover is net turnover (DemoANAF indicator I13), in RON.
	Turnover *float64
	// NetProfit is net profit for the year (DemoANAF I18 profit minus I19 loss), in RON.
	NetProfit *float64
	// Employees is the average number of employees (DemoANAF indicator I20).
	Employees *int
	// CAENDescription is the human-readable primary activity for that year, when supplied.
	CAENDescription string
}
