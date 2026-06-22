package agent

import (
	"context"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/reqmeta"
)

// fakeConsentRepo records consent rows it is asked to create and assigns ids.
type fakeConsentRepo struct {
	created []*domain.Consent
}

func (r *fakeConsentRepo) Create(_ context.Context, c *domain.Consent) error {
	c.ID = "consent-1"
	r.created = append(r.created, c)
	return nil
}

// newConsentCore wires a Core with the consent repo + version through the full
// flow constructor (Angrosist buyer flow by default).
func newConsentCore(llm ports.LLM, conv *mockConvRepo, comp *mockCompanyRepo,
	contact *mockContactRepo, lead *mockLeadRepo, sourcing *mockSourcingRepo,
	consent ports.ConsentRepo, audit ports.ActivityLogRepo, version string) *Core {
	return NewWithFlows(llm, conv, &mockMsgRepo{}, comp, contact, lead, sourcing,
		&mockVerifier{}, NewFlowRegistry(),
		Repos{Consent: consent},
		Notifications{ActivityLog: audit, ConsentTextVersion: version})
}

// TestSaveLead_CapturesConsent asserts the Angrosist buyer save_lead path captures
// GDPR consent for the newly created contact: a consents row (web channel + the
// threaded request IP + the configured text version), the active-consent pointer
// set on the contact, and a "consent.captured" audit row (no PII beyond ids).
func TestSaveLead_CapturesConsent(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{
		ID: "conv-1", Channel: "web", State: domain.StateQualifying, BotActive: true,
	}}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "ACME SRL", IsActive: true},
	}}
	contact := &mockContactRepo{}
	consent := &fakeConsentRepo{}
	audit := &fakeActivityLog{}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{saveLeadCall()}},
		{Text: "Cererea a fost înregistrată."},
	}}

	core := newConsentCore(llm, conv, comp, contact, &mockLeadRepo{}, &mockSourcingRepo{},
		consent, audit, "v3")

	// Thread a client IP through the context as the HTTP boundary would.
	ctx := reqmeta.WithClientIP(context.Background(), "203.0.113.7")
	if _, err := core.RunTurn(ctx, "conv-1", "gata"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	if len(consent.created) != 1 {
		t.Fatalf("expected exactly one consent row, got %d", len(consent.created))
	}
	c := consent.created[0]
	if c.ContactID != "contact-1" {
		t.Fatalf("consent contact id = %q, want contact-1", c.ContactID)
	}
	if c.Channel != domain.ConsentChannelWeb {
		t.Fatalf("consent channel = %q, want web", c.Channel)
	}
	if c.TextVersion != "v3" {
		t.Fatalf("consent text version = %q, want v3", c.TextVersion)
	}
	if c.IP != "203.0.113.7" {
		t.Fatalf("consent ip = %q, want 203.0.113.7", c.IP)
	}

	// The contact's active-consent pointer was set to the new consent.
	if contact.activeConsentFor["contact-1"] != "consent-1" {
		t.Fatalf("active consent pointer = %q, want consent-1", contact.activeConsentFor["contact-1"])
	}

	// A consent.captured audit row was written (ids only, no PII).
	row := audit.byAction("consent.captured")
	if row == nil {
		t.Fatal("expected a consent.captured activity log row")
	}
	if row.EntityType != "contact" || row.EntityID != "contact-1" {
		t.Fatalf("consent.captured entity = %s/%s, want contact/contact-1", row.EntityType, row.EntityID)
	}
	if _, hasIP := row.Meta["ip"]; hasIP {
		t.Fatal("consent.captured audit meta must not contain ip (no PII)")
	}
}

// TestSaveLead_NoConsentRepo_NoPanic asserts a nil consent repo is a safe no-op
// (legacy wiring): the lead still submits and no consent capture is attempted.
func TestSaveLead_NoConsentRepo_NoPanic(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{
		ID: "conv-1", Channel: "web", State: domain.StateQualifying, BotActive: true,
	}}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "ACME SRL", IsActive: true},
	}}
	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{saveLeadCall()}},
		{Text: "ok"},
	}}
	// Default New constructor wires no consent repo.
	core := newTestCore(llm, conv, &mockMsgRepo{}, comp, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{})

	if _, err := core.RunTurn(context.Background(), "conv-1", "gata"); err != nil {
		t.Fatalf("RunTurn with nil consent repo: %v", err)
	}
}
