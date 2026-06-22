package agent

import (
	"context"
	"log"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/reqmeta"
)

// captureConsent records a GDPR consent event for a newly created contact
// (SECURITY.md §7.1, audit catalog "consent_given"). It writes a consents row
// (text_version, channel, ip, given_at=now), points contacts.consent_id at it,
// and appends a "consent.captured" audit row (contact id only — no PII).
//
// It is BEST-EFFORT: every failure is logged (no PII) and never blocks the lead
// submission. The channel comes from the conversation (web|whatsapp). The IP is
// read from the request context (threaded via reqmeta from the HTTP boundary):
// it is present for web turns and empty for WhatsApp (no HTTP request IP), which
// is stored as SQL NULL. A nil consentRepo (legacy/test wiring) is a no-op.
func (c *Core) captureConsent(ctx context.Context, conv *domain.Conversation, contactID string) {
	if c.consentRepo == nil || contactID == "" {
		return
	}

	channel := domain.ConsentChannelWeb
	if conv != nil && conv.Channel == "whatsapp" {
		channel = domain.ConsentChannelWhatsApp
	}

	version := c.consentTextVersion
	if version == "" {
		version = domain.DefaultConsentTextVersion
	}

	consent := &domain.Consent{
		ContactID:   contactID,
		TextVersion: version,
		Channel:     channel,
		IP:          reqmeta.ClientIP(ctx), // empty for whatsapp -> NULL
	}
	if err := c.consentRepo.Create(ctx, consent); err != nil {
		log.Printf("consent: create failed contact=%s channel=%s: %v", contactID, channel, err)
		return
	}
	// Point the contact's active-consent pointer at this consent (deferred FK).
	if err := c.contactRepo.SetActiveConsent(ctx, contactID, consent.ID); err != nil {
		log.Printf("consent: set active pointer failed contact=%s: %v", contactID, err)
		// Continue: the consent row itself (the proof) is already persisted.
	}

	c.logActivity(ctx, domain.ActivityLog{
		ActorType:  "agent",
		Action:     "consent.captured",
		EntityType: "contact",
		EntityID:   contactID,
		Meta: map[string]any{
			"text_version": version,
			"channel":      channel,
		},
	})
}
