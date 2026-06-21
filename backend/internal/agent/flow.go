package agent

import (
	"context"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// Flow is the data-driven definition of one qualification path, keyed by
// (Vertical, Intent). It captures everything that differs between paths so the
// agent core stays generic (AI_AGENT_SPEC §1/§2/§3):
//
//   - RequiredFields: the ordered list of field keys the path must collect. It is
//     surfaced to the LLM via the prompt and is the contract for what "complete"
//     means; the prompt enumerates the same fields.
//   - Prompt: resolves the system prompt for a given language from the versioned
//     in-code prompt library.
//   - ToolDefs: the vendor-neutral tool declarations offered to the model for this
//     path (verify_company + handoff_to_human are shared; save_lead carries the
//     path's field schema).
//   - Submit: how save_lead persists the path's TYPED REQUEST (sourcing_request
//     for Angrosist buyer, listing for PalletClearance seller, buyer_profile for
//     PalletClearance buyer) plus the thin lead. This is the only per-vertical
//     persistence seam — adding a vertical = adding a Flow with its own Submit and
//     a sibling typed-request writer, never a core rewrite (invariant #5).
type Flow struct {
	Vertical       string
	Intent         string
	RequiredFields []string
	Prompt         func(lang string) string
	ToolDefs       func() []ports.ToolDef
	Submit         func(ctx context.Context, c *Core, conv *domain.Conversation, args map[string]any) (map[string]any, error)
}

// flowKey identifies a flow by vertical + intent.
type flowKey struct {
	vertical string
	intent   string
}

// FlowRegistry resolves the active Flow for a conversation's (vertical, intent).
// It is constructed once and injected into the core. Unknown combinations fall
// back to the Angrosist buyer flow so a conversation always has a flow (the
// default also preserves legacy demo behavior).
type FlowRegistry struct {
	flows map[flowKey]*Flow
}

// NewFlowRegistry builds the registry with the Phase-1 flows: Angrosist buyer,
// PalletClearance buyer, PalletClearance seller. Adding a vertical/intent is a
// single entry here plus its Flow definition and typed-request writer.
func NewFlowRegistry() *FlowRegistry {
	flows := map[flowKey]*Flow{}
	register := func(f *Flow) { flows[flowKey{f.Vertical, f.Intent}] = f }

	register(angrosistBuyerFlow())
	register(palletClearanceBuyerFlow())
	register(palletClearanceSellerFlow())

	return &FlowRegistry{flows: flows}
}

// For returns the Flow for a (vertical, intent), falling back to the Angrosist
// buyer flow when the pair is unknown. It never returns nil.
func (r *FlowRegistry) For(vertical, intent string) *Flow {
	if f, ok := r.flows[flowKey{vertical, intent}]; ok {
		return f
	}
	return r.flows[flowKey{domain.VerticalAngrosist, domain.IntentBuy}]
}

// --- Flow definitions ---------------------------------------------------------

// angrosistBuyerFlow reproduces the original single-vertical behavior exactly:
// the same required fields, the same RO/EN prompt, the same tool set (the legacy
// save_lead schema), and the sourcing_request writer.
func angrosistBuyerFlow() *Flow {
	return &Flow{
		Vertical: domain.VerticalAngrosist,
		Intent:   domain.IntentBuy,
		RequiredFields: []string{
			"product_name", "quantity", "unit", "delivery_location",
			"cui", "phone", "email",
		},
		Prompt: func(lang string) string {
			return resolvePrompt(domain.VerticalAngrosist, domain.IntentBuy, lang)
		},
		ToolDefs: angrosistBuyerToolDefs,
		Submit: func(ctx context.Context, c *Core, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
			return c.submitSourcingRequest(ctx, conv, args)
		},
	}
}

// palletClearanceBuyerFlow collects the buyer-feed fields and writes a
// buyer_profiles row + lead. CUI is opportunistic (no mandatory verify gate).
func palletClearanceBuyerFlow() *Flow {
	return &Flow{
		Vertical: domain.VerticalPalletClearance,
		Intent:   domain.IntentBuy,
		RequiredFields: []string{
			"categories", "volume", "countries", "near_expiry_ok",
			"subscribe", "phone", "email",
		},
		Prompt: func(lang string) string {
			return resolvePrompt(domain.VerticalPalletClearance, domain.IntentBuy, lang)
		},
		ToolDefs: pcBuyerToolDefs,
		Submit: func(ctx context.Context, c *Core, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
			return c.submitBuyerProfile(ctx, conv, args)
		},
	}
}

// palletClearanceSellerFlow collects the lot fields and writes a listings row +
// lead. The seller-photo BLOCKING gate is part A2; here save_lead is a normal
// submit.
func palletClearanceSellerFlow() *Flow {
	return &Flow{
		Vertical: domain.VerticalPalletClearance,
		Intent:   domain.IntentSell,
		RequiredFields: []string{
			"stock_type", "category", "quantity", "unit", "location",
			"expiry", "confidential", "cui", "phone", "email",
		},
		Prompt: func(lang string) string {
			return resolvePrompt(domain.VerticalPalletClearance, domain.IntentSell, lang)
		},
		ToolDefs: pcSellerToolDefs,
		Submit: func(ctx context.Context, c *Core, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
			return c.submitListing(ctx, conv, args)
		},
	}
}

// flowFor resolves the active flow for a conversation, guarding against a nil
// registry (older wirings/tests that constructed the core without one fall back
// to the Angrosist buyer flow).
func (c *Core) flowFor(conv *domain.Conversation) *Flow {
	if c.flows == nil {
		return angrosistBuyerFlow()
	}
	return c.flows.For(conv.Vertical, conv.Intent)
}

// ensureFlow panics with a clear message if a flow is unexpectedly nil; callers
// rely on flowFor never returning nil, so this only fires on a programming error.
func ensureFlow(f *Flow) *Flow {
	if f == nil {
		panic("agent: nil flow")
	}
	return f
}
