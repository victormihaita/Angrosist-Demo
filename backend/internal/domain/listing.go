package domain

import "time"

// Listing is the PalletClearance SELLER typed request (migration 019). It is a
// sibling of the thin lead (invariant #5): a new vertical/intent adds a typed
// request like this rather than reshaping sourcing_requests. One listing per
// lead (UNIQUE lead_id). The seller-photo hard gate is enforced by the flow
// engine in a later part (A2); this struct carries the listing fields only.
//
// Note: the seller's measurement unit (paleți/cutii/tone) is collected
// conversationally but has no column in migration 019; it is surfaced to staff
// via the conversation's extracted fields rather than persisted on the listing
// (the schema is forward-only/additive — a unit column would be a later migration).
type Listing struct {
	ID           string
	LeadID       string
	CompanyID    string
	CategoryID   string // optional; empty => NULL
	StockType    string
	Quantity     *float64
	Location     string
	Country      string
	Expiry       *time.Time
	TargetPrice  *float64
	Confidential bool
	Status       string
	CreatedAt    time.Time
}
