package domain

import "time"

type SourcingRequest struct {
	ID               string
	LeadID           string
	ProductName      string
	Quantity         *float64
	Unit             string
	DeliveryLocation string
	CreatedAt        time.Time
}
