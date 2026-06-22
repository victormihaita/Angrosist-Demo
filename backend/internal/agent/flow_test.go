package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// --- extra mocks for the new typed-request repos -----------------------------

type mockListingRepo struct {
	created  []*domain.Listing
	upserted []*domain.Listing
}

func (m *mockListingRepo) Create(ctx context.Context, l *domain.Listing) error {
	l.ID = "listing-1"
	m.created = append(m.created, l)
	return nil
}
func (m *mockListingRepo) UpsertByLead(ctx context.Context, l *domain.Listing) error {
	l.ID = "listing-1"
	m.upserted = append(m.upserted, l)
	return nil
}

type mockBuyerProfileRepo struct {
	upserted []*domain.BuyerProfile
}

func (m *mockBuyerProfileRepo) Upsert(ctx context.Context, p *domain.BuyerProfile) error {
	p.ID = "bp-1"
	m.upserted = append(m.upserted, p)
	return nil
}

// mockDocRepo is an in-memory DocumentRepo for the seller-photo gate tests. It
// tracks photo counts per (ownerType, ownerID, kind) and records reassign calls.
type mockDocRepo struct {
	// counts maps "ownerType/ownerID/kind" -> count for CountByOwnerKind.
	counts    map[string]int
	created   []*domain.Document
	reassigns []reassignCall
}

type reassignCall struct {
	fromType, fromID, toType, toID, kind string
}

func newMockDocRepo() *mockDocRepo { return &mockDocRepo{counts: map[string]int{}} }

func docKey(ownerType, ownerID, kind string) string {
	return ownerType + "/" + ownerID + "/" + kind
}

func (m *mockDocRepo) Create(ctx context.Context, d *domain.Document) error {
	d.ID = "doc-1"
	m.created = append(m.created, d)
	m.counts[docKey(d.OwnerType, d.OwnerID, d.Kind)]++
	return nil
}
func (m *mockDocRepo) ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*domain.Document, error) {
	return nil, nil
}
func (m *mockDocRepo) CountByOwnerKind(ctx context.Context, ownerType, ownerID, kind string) (int, error) {
	return m.counts[docKey(ownerType, ownerID, kind)], nil
}
func (m *mockDocRepo) Reassign(ctx context.Context, fromOwnerType, fromOwnerID, toOwnerType, toOwnerID, kind string) (int, error) {
	moved := m.counts[docKey(fromOwnerType, fromOwnerID, kind)]
	m.counts[docKey(fromOwnerType, fromOwnerID, kind)] = 0
	m.counts[docKey(toOwnerType, toOwnerID, kind)] += moved
	m.reassigns = append(m.reassigns, reassignCall{fromOwnerType, fromOwnerID, toOwnerType, toOwnerID, kind})
	return moved, nil
}

var _ ports.DocumentRepo = (*mockDocRepo)(nil)

// newFlowCore builds a core wired with the full flow registry + the new repos so
// the PalletClearance flows can persist. A nil docs falls back to a fresh empty
// mockDocRepo (no photos) so the seller-photo gate is exercised by default.
func newFlowCore(llm ports.LLM, conv *mockConvRepo, msg *mockMsgRepo, comp *mockCompanyRepo,
	contact *mockContactRepo, lead *mockLeadRepo, sourcing *mockSourcingRepo,
	listing ports.ListingRepo, buyerProfile ports.BuyerProfileRepo, docs ports.DocumentRepo,
	verifier ports.CompanyDataProvider) *Core {
	if docs == nil {
		docs = newMockDocRepo()
	}
	return NewWithFlows(llm, conv, msg, comp, contact, lead, sourcing, verifier,
		NewFlowRegistry(), Repos{Listing: listing, BuyerProfile: buyerProfile, Document: docs}, Notifications{})
}

// TestFlowSelection_SellerUsesSellerPromptAndTools asserts a conversation tagged
// palletclearance/sell gets the seller system prompt and the seller tool set.
func TestFlowSelection_SellerUsesSellerPromptAndTools(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{
		ID: "conv-1", State: domain.StateQualifying, BotActive: true,
		Vertical: domain.VerticalPalletClearance, Intent: domain.IntentSell, Language: domain.LocaleRO,
	}}
	msg := &mockMsgRepo{}
	llm := &scriptedLLM{responses: []*ports.LLMResponse{{Text: "Ce tip de stoc aveți?"}}}

	core := newFlowCore(llm, conv, msg, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockListingRepo{}, &mockBuyerProfileRepo{}, nil, &mockVerifier{})

	if _, err := core.RunTurn(context.Background(), "conv-1", "Vreau să vând un lot"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if got := llm.calls[0].System; got != promptPCSellerRO {
		t.Fatalf("expected seller RO prompt, got a different prompt")
	}
	// Seller tools must include a save_lead with stock_type in its schema.
	if !toolSchemaContains(llm.calls[0].Tools, toolSaveLead, "stock_type") {
		t.Fatalf("seller save_lead schema missing stock_type")
	}
}

// TestFlowSelection_BuyerUsesBuyerPromptAndTools asserts palletclearance/buy gets
// the buyer prompt + buyer tool set (categories field), distinct from the seller.
func TestFlowSelection_BuyerUsesBuyerPromptAndTools(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{
		ID: "conv-1", State: domain.StateQualifying, BotActive: true,
		Vertical: domain.VerticalPalletClearance, Intent: domain.IntentBuy, Language: domain.LocaleRO,
	}}
	msg := &mockMsgRepo{}
	llm := &scriptedLLM{responses: []*ports.LLMResponse{{Text: "Ce categorii vă interesează?"}}}

	core := newFlowCore(llm, conv, msg, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockListingRepo{}, &mockBuyerProfileRepo{}, nil, &mockVerifier{})

	if _, err := core.RunTurn(context.Background(), "conv-1", "Caut loturi"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if got := llm.calls[0].System; got != promptPCBuyerRO {
		t.Fatalf("expected buyer RO prompt, got a different prompt")
	}
	if !toolSchemaContains(llm.calls[0].Tools, toolSaveLead, "categories") {
		t.Fatalf("buyer save_lead schema missing categories")
	}
	if toolSchemaContains(llm.calls[0].Tools, toolSaveLead, "stock_type") {
		t.Fatalf("buyer save_lead schema unexpectedly has stock_type")
	}
}

// TestFlowSelection_DefaultIsAngrosistBuyer asserts an untagged conversation falls
// back to the Angrosist buyer flow (preserving legacy behavior).
func TestFlowSelection_DefaultIsAngrosistBuyer(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: true}}
	msg := &mockMsgRepo{}
	llm := &scriptedLLM{responses: []*ports.LLMResponse{{Text: "Bună!"}}}

	core := newFlowCore(llm, conv, msg, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockListingRepo{}, &mockBuyerProfileRepo{}, nil, &mockVerifier{})

	if _, err := core.RunTurn(context.Background(), "conv-1", "Salut"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if got := llm.calls[0].System; got != promptAngrosistBuyerRO {
		t.Fatalf("untagged conversation should use Angrosist buyer prompt")
	}
}

// TestSubmit_SellerWritesListing asserts the seller save_lead path persists a
// listing + lead via the mock repos (no DB).
func TestSubmit_SellerWritesListing(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{
		ID: "conv-1", State: domain.StateQualifying, BotActive: true,
		Vertical: domain.VerticalPalletClearance, Intent: domain.IntentSell,
	}}
	msg := &mockMsgRepo{}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "SELLER SRL", IsActive: true},
	}}
	listingRepo := &mockListingRepo{}
	leadRepo := &mockLeadRepo{}
	// At least one photo exists for the conversation, so the seller-photo gate opens.
	docs := newMockDocRepo()
	docs.counts[docKey(docOwnerConversation, "conv-1", docKindPhoto)] = 1

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{{ID: "save_lead", Name: "save_lead", Args: map[string]any{
			"stock_type": "overstock", "category": "alimente", "quantity": float64(10),
			"unit": "paleți", "location": "Cluj", "country": "RO", "expiry": "2026-12-31",
			"confidential": true, "cui": "12345678", "company_name": "SELLER SRL",
			"phone": "0712345678", "email": "vanzator@example.com",
		}}}},
		{Text: "Lotul a fost înregistrat."},
	}}

	core := newFlowCore(llm, conv, msg, comp, &mockContactRepo{}, leadRepo,
		&mockSourcingRepo{}, listingRepo, &mockBuyerProfileRepo{}, docs, &mockVerifier{})

	if _, err := core.RunTurn(context.Background(), "conv-1", "Gata, salvează"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(listingRepo.upserted) != 1 {
		t.Fatalf("expected 1 listing upserted, got %d", len(listingRepo.upserted))
	}
	l := listingRepo.upserted[0]
	if l.StockType != "overstock" || l.Country != "RO" || !l.Confidential {
		t.Fatalf("listing fields wrong: %+v", l)
	}
	if len(leadRepo.created) != 1 {
		t.Fatalf("expected 1 lead created, got %d", len(leadRepo.created))
	}
	if leadRepo.created[0].Vertical != domain.VerticalPalletClearance || leadRepo.created[0].Intent != domain.IntentSell {
		t.Fatalf("lead vertical/intent wrong: %+v", leadRepo.created[0])
	}
	// The conversation-scoped photo must be re-pointed to the durable listing.
	if len(docs.reassigns) != 1 {
		t.Fatalf("expected 1 photo reassign, got %d", len(docs.reassigns))
	}
	rc := docs.reassigns[0]
	if rc.fromType != docOwnerConversation || rc.fromID != "conv-1" ||
		rc.toType != docOwnerListing || rc.toID != l.ID || rc.kind != docKindPhoto {
		t.Fatalf("unexpected reassign: %+v", rc)
	}
	if n := docs.counts[docKey(docOwnerListing, l.ID, docKindPhoto)]; n != 1 {
		t.Fatalf("expected 1 photo attached to listing, got %d", n)
	}
}

// TestSubmit_SellerBlockedWithoutPhoto asserts the seller-photo BLOCKING gate:
// with zero photos the save_lead path returns blocked:"photo_required" and creates
// NO listing and NO lead.
func TestSubmit_SellerBlockedWithoutPhoto(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{
		ID: "conv-1", State: domain.StateQualifying, BotActive: true,
		Vertical: domain.VerticalPalletClearance, Intent: domain.IntentSell,
	}}
	msg := &mockMsgRepo{}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "SELLER SRL", IsActive: true},
	}}
	listingRepo := &mockListingRepo{}
	leadRepo := &mockLeadRepo{}
	docs := newMockDocRepo() // no photos

	args := map[string]any{
		"stock_type": "overstock", "category": "alimente", "quantity": float64(10),
		"unit": "paleți", "location": "Cluj", "country": "RO", "expiry": "2026-12-31",
		"confidential": true, "cui": "12345678", "company_name": "SELLER SRL",
		"phone": "0712345678", "email": "vanzator@example.com",
	}

	core := newFlowCore(&scriptedLLM{}, conv, msg, comp, &mockContactRepo{}, leadRepo,
		&mockSourcingRepo{}, listingRepo, &mockBuyerProfileRepo{}, docs, &mockVerifier{})

	out, err := core.submitListing(context.Background(), conv.conv, args)
	if err != nil {
		t.Fatalf("submitListing: %v", err)
	}
	if out["submitted"] != false || out["blocked"] != "photo_required" {
		t.Fatalf("expected blocked:photo_required, got %+v", out)
	}
	if len(listingRepo.upserted) != 0 || len(listingRepo.created) != 0 {
		t.Fatalf("listing must NOT be created when blocked: %+v", listingRepo)
	}
	if len(leadRepo.created) != 0 {
		t.Fatalf("lead must NOT be created when blocked: %d", len(leadRepo.created))
	}
	if len(docs.reassigns) != 0 {
		t.Fatalf("no reassign expected when blocked: %d", len(docs.reassigns))
	}
}

// TestSubmit_BuyerWritesProfile asserts the buyer save_lead path persists a buyer
// profile + lead via the mock repos (no DB).
func TestSubmit_BuyerWritesProfile(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{
		ID: "conv-1", State: domain.StateQualifying, BotActive: true,
		Vertical: domain.VerticalPalletClearance, Intent: domain.IntentBuy,
	}}
	msg := &mockMsgRepo{}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"87654321": {ID: "company-1", CUI: "87654321", Name: "BUYER SRL", IsActive: true},
	}}
	bpRepo := &mockBuyerProfileRepo{}
	leadRepo := &mockLeadRepo{}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{{ID: "save_lead", Name: "save_lead", Args: map[string]any{
			"categories": []any{"alimente", "băuturi"}, "volume": "2 camioane/lună",
			"countries": []any{"RO", "BG"}, "near_expiry_ok": true, "subscribe": true,
			"cui": "87654321", "company_name": "BUYER SRL",
			"phone": "0712345678", "email": "cumparator@example.com",
		}}}},
		{Text: "Profilul a fost înregistrat."},
	}}

	core := newFlowCore(llm, conv, msg, comp, &mockContactRepo{}, leadRepo,
		&mockSourcingRepo{}, &mockListingRepo{}, bpRepo, nil, &mockVerifier{})

	if _, err := core.RunTurn(context.Background(), "conv-1", "Gata, salvează"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(bpRepo.upserted) != 1 {
		t.Fatalf("expected 1 buyer profile upserted, got %d", len(bpRepo.upserted))
	}
	p := bpRepo.upserted[0]
	if !p.NearExpiryOK || !p.Subscribed || len(p.Countries) != 2 {
		t.Fatalf("buyer profile fields wrong: %+v", p)
	}
	if p.Vertical != domain.VerticalPalletClearance {
		t.Fatalf("buyer profile vertical wrong: %q", p.Vertical)
	}
	if len(leadRepo.created) != 1 || leadRepo.created[0].Intent != domain.IntentBuy {
		t.Fatalf("lead not created with buy intent: %+v", leadRepo.created)
	}
}

// toolSchemaContains reports whether the named tool's JSON-schema parameters
// contain the given property name (a cheap substring check on the raw schema).
func toolSchemaContains(tools []ports.ToolDef, name, property string) bool {
	for _, td := range tools {
		if td.Name == name {
			return strings.Contains(string(td.Parameters), property)
		}
	}
	return false
}
