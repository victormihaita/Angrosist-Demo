package domain

import "time"

// BuyerProfile is the PalletClearance BUYER standing-demand profile
// (migration 020). It is a sibling typed request of the thin lead (invariant
// #5) and seeds the Phase-2 matching feed: when a listing appears, buyers whose
// categories/countries overlap and who are subscribed are notified.
//
// Categories holds category ids (UUIDs). In Milestone 0 the agent collects free
// text categories conversationally; until a category-resolution step exists, the
// id list may be empty and the human-readable interest list is kept in the
// conversation's extracted fields. Countries holds ISO codes (RO active at launch).
type BuyerProfile struct {
	ID           string
	CompanyID    string
	Vertical     string
	Categories   []string
	Countries    []string
	NearExpiryOK bool
	Subscribed   bool
	CreatedAt    time.Time
}
