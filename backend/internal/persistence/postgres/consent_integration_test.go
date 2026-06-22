package postgres

import (
	"context"
	"testing"

	"github.com/angrosist/demo/internal/domain"
)

// TestConsentRepo_CreateAndSetActivePointer exercises the consent capture
// persistence against a real Postgres: a consents row is written (channel + ip +
// version), and contacts.consent_id is pointed at it (the deferred circular FK).
func TestConsentRepo_CreateAndSetActivePointer(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	pool := GetPool()

	var contactID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO contacts (name, email) VALUES ('Consent Person', $1) RETURNING id`,
		"consent-"+randSuffix(t)+"@example.com").Scan(&contactID); err != nil {
		t.Fatalf("insert contact: %v", err)
	}
	t.Cleanup(func() { _, _ = GetPool().Exec(context.Background(), `DELETE FROM contacts WHERE id = $1`, contactID) })

	consentRepo := NewConsentRepo()
	contactRepo := NewContactRepo()

	c := &domain.Consent{ContactID: contactID, TextVersion: "v2", Channel: domain.ConsentChannelWeb, IP: "198.51.100.22"}
	if err := consentRepo.Create(ctx, c); err != nil {
		t.Fatalf("Create consent: %v", err)
	}
	if c.ID == "" || c.GivenAt.IsZero() {
		t.Fatalf("Create should assign id + given_at: %+v", c)
	}
	if err := contactRepo.SetActiveConsent(ctx, contactID, c.ID); err != nil {
		t.Fatalf("SetActiveConsent: %v", err)
	}

	// The consents row persisted with the right channel/version/ip.
	var version, channel string
	var ip *string
	if err := pool.QueryRow(ctx, `SELECT text_version, channel, host(ip) FROM consents WHERE id = $1`, c.ID).
		Scan(&version, &channel, &ip); err != nil {
		t.Fatalf("read consent: %v", err)
	}
	if version != "v2" || channel != "web" || ip == nil || *ip != "198.51.100.22" {
		t.Fatalf("consent row mismatch: version=%q channel=%q ip=%v", version, channel, ip)
	}

	// The contact now points at this consent.
	var pointer *string
	if err := pool.QueryRow(ctx, `SELECT consent_id FROM contacts WHERE id = $1`, contactID).Scan(&pointer); err != nil {
		t.Fatalf("read contact pointer: %v", err)
	}
	if pointer == nil || *pointer != c.ID {
		t.Fatalf("contacts.consent_id = %v, want %s", pointer, c.ID)
	}

	// FindIDByEmail (used by erasure-by-email) resolves the same contact.
	var email string
	_ = pool.QueryRow(ctx, `SELECT email FROM contacts WHERE id = $1`, contactID).Scan(&email)
	got, err := contactRepo.FindIDByEmail(ctx, email)
	if err != nil {
		t.Fatalf("FindIDByEmail: %v", err)
	}
	if got != contactID {
		t.Fatalf("FindIDByEmail = %q, want %q", got, contactID)
	}
}

// TestConsentCapture_WhatsAppFirstContact asserts the WhatsApp first-contact path
// (GetOrCreateByChannelPhone) captures consent atomically: the newly created
// contact gets a consents row (channel='whatsapp', no IP) and contacts.consent_id
// points at it. A second inbound from the same phone reuses the contact and does
// NOT create a second consent.
func TestConsentCapture_WhatsAppFirstContact(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	pool := GetPool()
	repo := NewConversationRepo()

	phone := "+40700" + randSuffix(t)

	conv, err := repo.GetOrCreateByChannelPhone(ctx, "whatsapp", phone, "angrosist", "buy")
	if err != nil {
		t.Fatalf("GetOrCreateByChannelPhone: %v", err)
	}

	// Resolve the created contact via the phone.
	var contactID string
	if err := pool.QueryRow(ctx, `SELECT id FROM contacts WHERE phone = $1`, phone).Scan(&contactID); err != nil {
		t.Fatalf("read contact by phone: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = GetPool().Exec(c, `DELETE FROM conversations WHERE id = $1`, conv.ID)
		_, _ = GetPool().Exec(c, `DELETE FROM contacts WHERE id = $1`, contactID)
	})

	// Exactly one whatsapp consent row, with NULL ip, and the contact points at it.
	var n int
	var channel string
	var ipNull bool
	var pointer *string
	if err := pool.QueryRow(ctx, `
		SELECT count(*), max(channel), bool_and(ip IS NULL) FROM consents WHERE contact_id = $1`, contactID).
		Scan(&n, &channel, &ipNull); err != nil {
		t.Fatalf("read consents: %v", err)
	}
	if n != 1 || channel != "whatsapp" || !ipNull {
		t.Fatalf("whatsapp consent mismatch: n=%d channel=%q ipNull=%v", n, channel, ipNull)
	}
	if err := pool.QueryRow(ctx, `SELECT consent_id FROM contacts WHERE id = $1`, contactID).Scan(&pointer); err != nil {
		t.Fatalf("read pointer: %v", err)
	}
	if pointer == nil {
		t.Fatal("contacts.consent_id should be set on whatsapp first contact")
	}

	// Second inbound reuses the contact -> still exactly one consent.
	if _, err := repo.GetOrCreateByChannelPhone(ctx, "whatsapp", phone, "angrosist", "buy"); err != nil {
		t.Fatalf("second GetOrCreateByChannelPhone: %v", err)
	}
	var n2 int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM consents WHERE contact_id = $1`, contactID).Scan(&n2); err != nil {
		t.Fatalf("recount consents: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("expected 1 consent after redelivery, got %d", n2)
	}
}
