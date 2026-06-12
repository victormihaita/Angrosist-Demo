package postgres

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
)

type LeadRepo struct{}

func NewLeadRepo() *LeadRepo { return &LeadRepo{} }

func (r *LeadRepo) Create(ctx context.Context, lead *domain.Lead) error {
	row := GetPool().QueryRow(ctx, `
		INSERT INTO leads (conversation_id, company_id, contact_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, lead.ConversationID, nullStr(lead.CompanyID), nullStr(lead.ContactID), lead.Status)
	return row.Scan(&lead.ID, &lead.CreatedAt)
}

func (r *LeadRepo) List(ctx context.Context) ([]*domain.LeadSummary, error) {
	rows, err := GetPool().Query(ctx, `
		SELECT
			l.id,
			l.status,
			COALESCE(c.name, '') AS company_name,
			COALESCE(c.cui, '') AS cui,
			COALESCE(sr.product_name, '') AS product_name,
			sr.quantity,
			COALESCE(sr.unit, '') AS unit,
			COALESCE(sr.delivery_location, '') AS delivery_location,
			l.created_at
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		ORDER BY l.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []*domain.LeadSummary
	for rows.Next() {
		var s domain.LeadSummary
		if err := rows.Scan(
			&s.ID, &s.Status, &s.CompanyName, &s.CUI,
			&s.ProductName, &s.Quantity, &s.Unit, &s.DeliveryLocation,
			&s.CreatedAt,
		); err != nil {
			return nil, err
		}
		leads = append(leads, &s)
	}
	return leads, rows.Err()
}

func (r *LeadRepo) GetByID(ctx context.Context, id string) (*domain.LeadDetail, error) {
	row := GetPool().QueryRow(ctx, `
		SELECT
			l.id,
			l.status,
			COALESCE(c.name, '') AS company_name,
			COALESCE(c.cui, '') AS cui,
			COALESCE(sr.product_name, '') AS product_name,
			sr.quantity,
			COALESCE(sr.unit, '') AS unit,
			COALESCE(sr.delivery_location, '') AS delivery_location,
			l.created_at,
			COALESCE(c.address, '') AS address,
			COALESCE(c.county, '') AS county,
			COALESCE(ct.phone, '') AS phone,
			COALESCE(ct.email, '') AS email,
			l.conversation_id
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN contacts ct ON ct.id = l.contact_id
		LEFT JOIN sourcing_requests sr ON sr.lead_id = l.id
		WHERE l.id = $1
	`, id)

	var d domain.LeadDetail
	var convID string
	err := row.Scan(
		&d.ID, &d.Status, &d.CompanyName, &d.CUI,
		&d.ProductName, &d.Quantity, &d.Unit, &d.DeliveryLocation,
		&d.CreatedAt, &d.Address, &d.County, &d.Phone, &d.Email, &convID,
	)
	if err != nil {
		return nil, err
	}

	msgRepo := NewMessageRepo()
	msgs, err := msgRepo.ListByConversation(ctx, convID)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		d.Transcript = append(d.Transcript, *m)
	}
	return &d, nil
}
