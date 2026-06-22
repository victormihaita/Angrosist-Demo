package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
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

// SetActiveConsent points contacts.consent_id at the given consent (the deferred
// circular FK from migration 015 — the contact's current consent). A missing
// contact returns ports.ErrNotFound.
func (r *ContactRepo) SetActiveConsent(ctx context.Context, contactID, consentID string) error {
	tag, err := GetPool().Exec(ctx, `
		UPDATE contacts SET consent_id = $2::uuid, updated_at = now() WHERE id = $1::uuid
	`, contactID, consentID)
	if err != nil {
		return fmt.Errorf("set active consent (contact=%s): %w", contactID, err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// FindIDByEmail returns the most recent contact id carrying email (matched
// case-insensitively), or ports.ErrNotFound when none exists. It backs erasure by
// email; the email is never logged.
func (r *ContactRepo) FindIDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := GetPool().QueryRow(ctx, `
		SELECT id FROM contacts
		WHERE email IS NOT NULL AND lower(email) = lower($1)
		ORDER BY created_at DESC
		LIMIT 1
	`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ports.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find contact by email: %w", err)
	}
	return id, nil
}

var _ ports.ContactRepo = (*ContactRepo)(nil)
