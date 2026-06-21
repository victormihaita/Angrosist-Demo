package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/angrosist/demo/internal/domain"
)

// This file holds the per-flow save_lead persistence (the Flow.Submit seam). Each
// function creates/updates the thin lead and ITS typed request, never reshaping
// another vertical's request (invariant #5). Adding a vertical = adding one more
// submit function + a sibling typed-request writer, with no change to the others.

// resolveCompany finds the verified company by CUI, or — when ANAF was
// unavailable / the CUI is absent — upserts a minimal record from the agent's
// args so the lead can still be written (graceful degradation, AI_AGENT_SPEC §8.2).
// It returns (nil, nil) when no CUI at all is available (the company is optional
// for the PalletClearance buyer flow).
func (c *Core) resolveCompany(ctx context.Context, args map[string]any) (*domain.Company, error) {
	cui := strArg(args["cui"])
	if cui == "" {
		return nil, nil
	}
	company, err := c.companyRepo.GetByCUI(ctx, cui)
	if err == nil {
		return company, nil
	}
	// Not cached (ANAF unavailable or not yet verified) — upsert a minimal record.
	company = &domain.Company{
		CUI:      cui,
		Country:  "RO",
		RegNo:    cui,
		Name:     strArg(args["company_name"]),
		IsActive: true,
	}
	if upsertErr := c.companyRepo.Upsert(ctx, company); upsertErr != nil {
		return nil, fmt.Errorf("upsert company: %w", upsertErr)
	}
	company, err = c.companyRepo.GetByCUI(ctx, cui)
	if err != nil {
		return nil, fmt.Errorf("company not found after upsert: %w", err)
	}
	return company, nil
}

// upsertLeadAndContact creates (or updates in place) the thin lead + its contact
// for a conversation, tagging the lead with the flow's vertical/intent. It returns
// the lead id and whether an existing lead was updated. companyID may be empty
// (no company on file, e.g. PC buyer without CUI).
func (c *Core) upsertLeadAndContact(ctx context.Context, conv *domain.Conversation, flow *Flow, companyID, phone, email string) (leadID string, updated bool, err error) {
	if existing, e := c.leadRepo.GetByConversationID(ctx, conv.ID); e == nil && existing != nil {
		contact := &domain.Contact{
			ID:        existing.ContactID,
			CompanyID: companyID,
			Phone:     phone,
			Email:     email,
		}
		if err := c.contactRepo.Update(ctx, contact); err != nil {
			return "", false, fmt.Errorf("update contact: %w", err)
		}
		if err := c.leadRepo.UpdateCompanyContact(ctx, existing.ID, companyID, contact.ID); err != nil {
			return "", false, fmt.Errorf("update lead: %w", err)
		}
		return existing.ID, true, nil
	}

	contact := &domain.Contact{CompanyID: companyID, Phone: phone, Email: email}
	if err := c.contactRepo.Create(ctx, contact); err != nil {
		return "", false, fmt.Errorf("create contact: %w", err)
	}
	lead := &domain.Lead{
		ConversationID: conv.ID,
		CompanyID:      companyID,
		ContactID:      contact.ID,
		Status:         "new",
		Vertical:       flow.Vertical,
		Intent:         flow.Intent,
	}
	if err := c.leadRepo.Create(ctx, lead); err != nil {
		return "", false, fmt.Errorf("create lead: %w", err)
	}
	return lead.ID, false, nil
}

// --- Angrosist buyer: sourcing_request ---------------------------------------

// submitSourcingRequest is the Angrosist buyer save_lead path. It is the original
// toolSaveLead, refactored onto the shared helpers; behavior (and the notify +
// extracted-field updates) is unchanged.
func (c *Core) submitSourcingRequest(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	flow := angrosistBuyerFlow()
	phone := strArg(args["phone"])
	email := strArg(args["email"])

	company, err := c.resolveCompany(ctx, args)
	if err != nil {
		return nil, err
	}
	companyID := ""
	if company != nil {
		companyID = company.ID
	}

	leadID, updated, err := c.upsertLeadAndContact(ctx, conv, flow, companyID, phone, email)
	if err != nil {
		return nil, err
	}

	req := &domain.SourcingRequest{
		LeadID:           leadID,
		ProductName:      strArg(args["product_name"]),
		Quantity:         extractFloat(args["quantity"]),
		Unit:             strArg(args["unit"]),
		DeliveryLocation: strArg(args["delivery_location"]),
	}
	if updated {
		if err := c.sourcingRepo.UpdateByLeadID(ctx, req); err != nil {
			return nil, fmt.Errorf("update sourcing request: %w", err)
		}
	} else {
		if err := c.sourcingRepo.Create(ctx, req); err != nil {
			return nil, fmt.Errorf("create sourcing request: %w", err)
		}
	}

	c.updateExtractedSourcing(ctx, conv, req, phone, email, args)
	c.notifyLeadSubmitted(ctx, conv, leadID, company, req, phone, email)

	out := map[string]any{"saved": true, "lead_id": leadID}
	if updated {
		out["updated"] = true
	}
	return out, nil
}

func (c *Core) updateExtractedSourcing(ctx context.Context, conv *domain.Conversation, req *domain.SourcingRequest, phone, email string, args map[string]any) {
	extracted := conv.Extracted
	if extracted == nil {
		extracted = make(map[string]any)
	}
	extracted["product_name"] = req.ProductName
	extracted["quantity"] = args["quantity"]
	extracted["unit"] = req.Unit
	extracted["delivery_location"] = req.DeliveryLocation
	extracted["phone"] = phone
	extracted["email"] = email
	_ = c.convRepo.UpdateExtracted(ctx, conv.ID, extracted)
	_ = c.convRepo.UpdateState(ctx, conv.ID, domain.StateConfirmed)
	conv.Extracted = extracted
}

// --- PalletClearance seller: listing -----------------------------------------

// submitListing is the PalletClearance seller save_lead path: it writes a
// listings row (sibling typed request) + the thin lead. The seller-photo BLOCKING
// gate is part A2; here it is a normal submit.
func (c *Core) submitListing(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	if c.listingRepo == nil {
		return map[string]any{"saved": false, "reason": "listing_repo_unavailable"}, nil
	}
	flow := palletClearanceSellerFlow()
	phone := strArg(args["phone"])
	email := strArg(args["email"])

	company, err := c.resolveCompany(ctx, args)
	if err != nil {
		return nil, err
	}
	companyID := ""
	if company != nil {
		companyID = company.ID
	}

	leadID, updated, err := c.upsertLeadAndContact(ctx, conv, flow, companyID, phone, email)
	if err != nil {
		return nil, err
	}

	listing := &domain.Listing{
		LeadID:       leadID,
		CompanyID:    companyID,
		StockType:    strArg(args["stock_type"]),
		Quantity:     extractFloat(args["quantity"]),
		Location:     strArg(args["location"]),
		Country:      strArg(args["country"]),
		Expiry:       parseDate(args["expiry"]),
		TargetPrice:  extractFloat(args["target_price"]),
		Confidential: boolArg(args["confidential"]),
		Status:       "active",
	}
	if err := c.listingRepo.UpsertByLead(ctx, listing); err != nil {
		return nil, fmt.Errorf("upsert listing: %w", err)
	}

	c.updateExtractedSeller(ctx, conv, args, phone, email)

	out := map[string]any{"saved": true, "lead_id": leadID}
	if updated {
		out["updated"] = true
	}
	return out, nil
}

func (c *Core) updateExtractedSeller(ctx context.Context, conv *domain.Conversation, args map[string]any, phone, email string) {
	extracted := conv.Extracted
	if extracted == nil {
		extracted = make(map[string]any)
	}
	extracted["stock_type"] = strArg(args["stock_type"])
	extracted["category"] = strArg(args["category"])
	extracted["quantity"] = args["quantity"]
	extracted["unit"] = strArg(args["unit"])
	extracted["location"] = strArg(args["location"])
	extracted["expiry"] = strArg(args["expiry"])
	extracted["confidential"] = boolArg(args["confidential"])
	extracted["phone"] = phone
	extracted["email"] = email
	_ = c.convRepo.UpdateExtracted(ctx, conv.ID, extracted)
	_ = c.convRepo.UpdateState(ctx, conv.ID, domain.StateConfirmed)
	conv.Extracted = extracted
}

// --- PalletClearance buyer: buyer_profile ------------------------------------

// submitBuyerProfile is the PalletClearance buyer save_lead path: it writes a
// buyer_profiles row (sibling typed request) + the thin lead. CUI is opportunistic
// — the lead is written even without a company on file. A buyer_profiles row
// requires a company (its company_id is NOT NULL), so the profile is written only
// when a company is available; otherwise just the lead is recorded for staff.
func (c *Core) submitBuyerProfile(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	if c.buyerProfileRepo == nil {
		return map[string]any{"saved": false, "reason": "buyer_profile_repo_unavailable"}, nil
	}
	flow := palletClearanceBuyerFlow()
	phone := strArg(args["phone"])
	email := strArg(args["email"])

	company, err := c.resolveCompany(ctx, args)
	if err != nil {
		return nil, err
	}
	companyID := ""
	if company != nil {
		companyID = company.ID
	}

	leadID, updated, err := c.upsertLeadAndContact(ctx, conv, flow, companyID, phone, email)
	if err != nil {
		return nil, err
	}

	profileWritten := false
	if companyID != "" {
		profile := &domain.BuyerProfile{
			CompanyID:    companyID,
			Vertical:     domain.VerticalPalletClearance,
			Categories:   []string{}, // free-text categories are kept in extracted until a resolver lands
			Countries:    strSliceArg(args["countries"]),
			NearExpiryOK: boolArg(args["near_expiry_ok"]),
			Subscribed:   boolArg(args["subscribe"]),
		}
		if err := c.buyerProfileRepo.Upsert(ctx, profile); err != nil {
			return nil, fmt.Errorf("upsert buyer profile: %w", err)
		}
		profileWritten = true
	}

	c.updateExtractedBuyerProfile(ctx, conv, args, phone, email)

	out := map[string]any{"saved": true, "lead_id": leadID, "profile_saved": profileWritten}
	if updated {
		out["updated"] = true
	}
	return out, nil
}

func (c *Core) updateExtractedBuyerProfile(ctx context.Context, conv *domain.Conversation, args map[string]any, phone, email string) {
	extracted := conv.Extracted
	if extracted == nil {
		extracted = make(map[string]any)
	}
	extracted["categories"] = strSliceArg(args["categories"])
	extracted["volume"] = strArg(args["volume"])
	extracted["countries"] = strSliceArg(args["countries"])
	extracted["near_expiry_ok"] = boolArg(args["near_expiry_ok"])
	extracted["subscribe"] = boolArg(args["subscribe"])
	extracted["phone"] = phone
	extracted["email"] = email
	_ = c.convRepo.UpdateExtracted(ctx, conv.ID, extracted)
	_ = c.convRepo.UpdateState(ctx, conv.ID, domain.StateConfirmed)
	conv.Extracted = extracted
}

// parseDate coerces a tool arg into a *time.Time, accepting ISO YYYY-MM-DD (the
// format the prompt requests) or RFC3339. Unparseable / empty values yield nil so
// the typed request stores a NULL expiry rather than failing the submit.
func parseDate(v any) *time.Time {
	s := strArg(v)
	if s == "" {
		return nil
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
