package domain

import "time"

type Company struct {
	ID          string
	CUI         string
	Name        string
	Address     string
	County      string
	IsActive    bool
	RawData     []byte // full ANAF JSON response
	VerifiedAt  *time.Time
	CreatedAt   time.Time
}
