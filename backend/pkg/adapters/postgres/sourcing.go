package postgres

import (
	"context"

	"github.com/angrosist/demo/pkg/domain"
)

type SourcingRepo struct{}

func NewSourcingRepo() *SourcingRepo { return &SourcingRepo{} }

func (r *SourcingRepo) Create(ctx context.Context, req *domain.SourcingRequest) error {
	row := GetPool().QueryRow(ctx, `
		INSERT INTO sourcing_requests (lead_id, product_name, quantity, unit, delivery_location)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, req.LeadID, req.ProductName, req.Quantity, nullStr(req.Unit), nullStr(req.DeliveryLocation))
	return row.Scan(&req.ID, &req.CreatedAt)
}

func (r *SourcingRepo) UpdateByLeadID(ctx context.Context, req *domain.SourcingRequest) error {
	_, err := GetPool().Exec(ctx, `
		UPDATE sourcing_requests
		SET product_name = $2, quantity = $3, unit = $4, delivery_location = $5
		WHERE lead_id = $1
	`, req.LeadID, req.ProductName, req.Quantity, nullStr(req.Unit), nullStr(req.DeliveryLocation))
	return err
}
