package domain

import "time"

// Cursor is the opaque keyset position used by cursor pagination. It captures the
// (CreatedAt, ID) of the last item on a page so the next page resumes strictly
// after it under the canonical `created_at DESC, id DESC` ordering. It is encoded
// to/decoded from an opaque base64 token at the API boundary (no infra here).
type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

// LeadFilter narrows the leads pipeline list. Empty fields are ignored. Limit is
// already clamped by the caller to the valid 1..100 range; After is the keyset
// position (nil = first page).
type LeadFilter struct {
	Status     string  // exact match on leads.status (validated against lead_statuses)
	Vertical   string  // exact match on leads.vertical
	AssignedTo *string // pointer so "" (unassigned filter) differs from absent; nil = no filter
	Query      string  // ILIKE on company name / product name
	NeedsHuman *bool   // true restricts to the handoff queue (uses the partial index)
	Limit      int     // page size (already clamped)
	After      *Cursor // keyset position; nil = first page
}

// CompanyFilter narrows the B2B directory list. Empty fields are ignored.
type CompanyFilter struct {
	Role    string  // array-containment match on companies.roles[] (GIN)
	Country string  // exact match on companies.country
	Query   string  // ILIKE on company name
	Limit   int     // page size (already clamped)
	After   *Cursor // keyset position; nil = first page
}

// CompanyFinancialView is one year's financial snapshot embedded in a CompanyDetail.
type CompanyFinancialView struct {
	Year            int      `json:"year"`
	Turnover        *float64 `json:"turnover"`
	NetProfit       *float64 `json:"net_profit"`
	Employees       *int     `json:"employees"`
	CAENDescription string   `json:"caen_description,omitempty"`
}

// CompanySummary is the directory list-view projection.
type CompanySummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CUI       string    `json:"cui"`
	Country   string    `json:"country"`
	RegNo     string    `json:"reg_no"`
	CAEN      string    `json:"caen"`
	VATStatus string    `json:"vat_status"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

// CompanyDetail is the directory detail view: the summary plus address/county,
// the richer ONRC identity fields, administrators, the latest verification, and
// any financial-year snapshots.
type CompanyDetail struct {
	CompanySummary
	Address string `json:"address"`
	County  string `json:"county"`
	// RegistrationNumber is the ONRC J-number (e.g. "J40/372/2002").
	RegistrationNumber string `json:"registration_number,omitempty"`
	// RegistrationDate is the incorporation date, omitted when unknown.
	RegistrationDate *time.Time `json:"registration_date,omitempty"`
	// LegalForm is the legal form (SA / SRL / ...).
	LegalForm string `json:"legal_form,omitempty"`
	// EFactura reports e-Factura (e-invoicing) registration.
	EFactura bool `json:"e_factura"`
	// AuthorizedCAEN holds all permitted activity codes when available.
	AuthorizedCAEN []string `json:"authorized_caen,omitempty"`
	// Administrators are the company's administrators' names (from the latest verification).
	Administrators []string `json:"administrators,omitempty"`
	IsActive       bool     `json:"is_active"`

	Verification *CompanyVerificationView `json:"verification,omitempty"`
	Financials   []CompanyFinancialView   `json:"financials,omitempty"`
}

// HandoffItem is one entry in the human-handoff queue: the lead facets a
// consultant needs to triage, plus a short snippet of the last message.
type HandoffItem struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Vertical    string    `json:"vertical"`
	CompanyName string    `json:"company_name"`
	ProductName string    `json:"product_name"`
	AssignedTo  *string   `json:"assigned_to"`
	LastMessage string    `json:"last_message"`
	CreatedAt   time.Time `json:"created_at"`
}

// OfferUpdate carries a manual offer-tracking change. Nil fields are left
// unchanged. Status is validated against the lead_statuses lookup; Value must be
// non-negative; Note is length-bounded — all enforced in the use-case/handler.
type OfferUpdate struct {
	Status *string
	Value  *float64
	Note   *string
}

// KPIs is the dashboard summary: counts and pipeline value with documented
// definitions (see KPIUseCase / the repo query).
type KPIs struct {
	// OffersSent counts leads in an offer-bearing status (offer_sent, negotiation, won).
	OffersSent int `json:"offers_sent"`
	// Won counts leads in the terminal 'won' status.
	Won int `json:"won"`
	// Qualified counts leads that reached qualification or beyond (qualified and
	// every downstream status), the denominator for the conversion rate.
	Qualified int `json:"qualified"`
	// ConversionRate is Won / Qualified (0 when Qualified is 0). Range 0..1.
	ConversionRate float64 `json:"conversion_rate"`
	// PipelineValue sums offer_value over open (non-terminal) statuses.
	PipelineValue float64 `json:"pipeline_value"`
}

// ActivityLog is one append-only audit row (migration 022). It is written on
// meaningful dashboard mutations (offer updates, assignment).
type ActivityLog struct {
	ActorType  string         // 'staff' | 'admin' | 'agent' | 'system'
	ActorID    string         // user id (empty for system/agent)
	Action     string         // e.g. 'offer.update', 'lead.assign'
	EntityType string         // 'lead' | 'company' | ...
	EntityID   string         // subject id
	Meta       map[string]any // structured diff/context (no PII)
}
