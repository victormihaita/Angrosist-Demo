package domain

import "time"

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
}

// LeadDetail includes the full message transcript.
type LeadDetail struct {
	LeadSummary
	Address    string    `json:"address"`
	County     string    `json:"county"`
	Phone      string    `json:"phone"`
	Email      string    `json:"email"`
	Transcript []Message `json:"transcript"`
}
