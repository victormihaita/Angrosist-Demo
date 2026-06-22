package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// consentTextVersion resolves the consent text/version to record. It reads
// CONSENT_TEXT_VERSION and falls back to domain.DefaultConsentTextVersion when
// unset, so the NOT NULL consents.text_version column is always satisfied. It is
// used by the WhatsApp first-contact capture inside GetOrCreateByChannelPhone
// (which has no Core/Notifications to thread the version through); the web/lead
// submit path threads the version from config via the agent Core instead.
func consentTextVersion() string {
	if v := strings.TrimSpace(os.Getenv("CONSENT_TEXT_VERSION")); v != "" {
		return v
	}
	return domain.DefaultConsentTextVersion
}

// ConsentRepo is the Postgres adapter for the consents table (migration 015,
// invariant #7 / NFR-3). Consent is personal data and CASCADE-deletes with its
// contact. All statements are parameterized; the IP is stored, never logged.
type ConsentRepo struct{}

// NewConsentRepo constructs the Postgres-backed consent repository.
func NewConsentRepo() *ConsentRepo { return &ConsentRepo{} }

// Create inserts one consent row, writing the assigned id/given_at/created_at back
// onto c. An empty TextVersion falls back to domain.DefaultConsentTextVersion so
// the NOT NULL column is always satisfied. An empty IP is stored as SQL NULL
// (the WhatsApp path); the ip column is INET, so $4 is cast accordingly.
func (r *ConsentRepo) Create(ctx context.Context, c *domain.Consent) error {
	if c.TextVersion == "" {
		c.TextVersion = domain.DefaultConsentTextVersion
	}
	row := GetPool().QueryRow(ctx, `
		INSERT INTO consents (contact_id, text_version, channel, ip)
		VALUES ($1::uuid, $2, $3, $4::inet)
		RETURNING id, given_at, created_at
	`, c.ContactID, c.TextVersion, c.Channel, nullStr(c.IP))
	if err := row.Scan(&c.ID, &c.GivenAt, &c.CreatedAt); err != nil {
		return fmt.Errorf("create consent (contact=%s channel=%s): %w", c.ContactID, c.Channel, err)
	}
	return nil
}

var _ ports.ConsentRepo = (*ConsentRepo)(nil)
