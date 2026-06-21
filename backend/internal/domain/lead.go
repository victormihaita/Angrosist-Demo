package domain

import (
	"encoding/json"
	"time"
)

type Lead struct {
	ID             string
	ConversationID string
	CompanyID      string
	ContactID      string
	Status         string
	CreatedAt      time.Time
}

type Contact struct {
	ID        string
	CompanyID string
	Name      string
	Phone     string
	Email     string
	CreatedAt time.Time
}

// LeadSummary is the list-view projection used by the dashboard table.
type LeadSummary struct {
	ID               string    `json:"id"`
	Status           string    `json:"status"`
	CompanyName      string    `json:"company_name"`
	CUI              string    `json:"cui"`
	ProductName      string    `json:"product_name"`
	Quantity         *float64  `json:"quantity"`
	Unit             string    `json:"unit"`
	DeliveryLocation string    `json:"delivery_location"`
	CreatedAt        time.Time `json:"created_at"`

	// Dashboard pipeline fields (migration 016/025). AssignedTo is the user id
	// that owns the lead (nil = unassigned); Vertical scopes the pipeline;
	// OfferValue/OfferNote carry manual offer tracking.
	Vertical   string   `json:"vertical"`
	AssignedTo *string  `json:"assigned_to"`
	NeedsHuman bool     `json:"needs_human"`
	OfferValue *float64 `json:"offer_value"`
	OfferNote  string   `json:"offer_note,omitempty"`
}

// SourcingRequestView is the typed-request projection embedded in a LeadDetail.
// It is nullable: a lead may exist before its sourcing_request is written.
type SourcingRequestView struct {
	ID               string    `json:"id"`
	Product          string    `json:"product"`
	Quantity         *float64  `json:"quantity"`
	Unit             string    `json:"unit"`
	DeliveryLocation string    `json:"delivery_location"`
	Recurring        bool      `json:"recurring"`
	Budget           *float64  `json:"budget"`
	CreatedAt        time.Time `json:"created_at"`
}

// CompanyVerificationView is the latest verification snapshot for a lead's company.
type CompanyVerificationView struct {
	Source         string          `json:"source"`
	VATStatus      string          `json:"vat_status"`
	Administrators json.RawMessage `json:"administrators,omitempty"`
	CheckedAt      time.Time       `json:"checked_at"`
}

// LeadCompanyView is the company facet embedded in a LeadDetail.
type LeadCompanyView struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	CUI          string                   `json:"cui"`
	Country      string                   `json:"country"`
	RegNo        string                   `json:"reg_no"`
	CAEN         string                   `json:"caen"`
	VATStatus    string                   `json:"vat_status"`
	Roles        []string                 `json:"roles"`
	Verification *CompanyVerificationView `json:"verification,omitempty"`
}

// LeadContactView is the contact facet embedded in a LeadDetail.
type LeadContactView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

// LeadDetail includes the full message transcript plus the typed request,
// company (with latest verification) and contact facets.
type LeadDetail struct {
	LeadSummary
	Address    string    `json:"address"`
	County     string    `json:"county"`
	Phone      string    `json:"phone"`
	Email      string    `json:"email"`
	Transcript []Message `json:"transcript"`

	SourcingRequest *SourcingRequestView `json:"sourcing_request,omitempty"`
	Company         *LeadCompanyView     `json:"company,omitempty"`
	Contact         *LeadContactView     `json:"contact,omitempty"`
}
