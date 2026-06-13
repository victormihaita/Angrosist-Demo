package postgres

import (
	"context"

	"github.com/angrosist/demo/pkg/domain"
)

type ContactRepo struct{}

func NewContactRepo() *ContactRepo { return &ContactRepo{} }

func (r *ContactRepo) Create(ctx context.Context, contact *domain.Contact) error {
	row := GetPool().QueryRow(ctx, `
		INSERT INTO contacts (company_id, name, phone, email)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, nullStr(contact.CompanyID), nullStr(contact.Name), nullStr(contact.Phone), nullStr(contact.Email))
	return row.Scan(&contact.ID, &contact.CreatedAt)
}

func (r *ContactRepo) Update(ctx context.Context, contact *domain.Contact) error {
	_, err := GetPool().Exec(ctx, `
		UPDATE contacts SET company_id = $2, phone = $3, email = $4 WHERE id = $1
	`, contact.ID, nullStr(contact.CompanyID), nullStr(contact.Phone), nullStr(contact.Email))
	return err
}
