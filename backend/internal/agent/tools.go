package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// Tool names used by the agent. Kept identical to the prior Gemini runner so the
// behavior and persisted transcript shape are unchanged.
const (
	toolVerifyCompany = "verify_company"
	toolSaveLead      = "save_lead"
	toolHandoff       = "handoff_to_human"
)

// handoffReasons is the documented enum for handoff_to_human.reason (AI_AGENT_SPEC
// §5.4). Args are re-validated server-side: an unknown reason is normalized to
// "other" so a jailbroken model cannot smuggle arbitrary values.
var handoffReasons = map[string]bool{
	"user_request":               true,
	"confusion_or_contradiction": true,
	"out_of_scope":               true,
	"verification_failed":        true,
	"unclassifiable_need":        true,
	"other":                      true,
}

// toolDefs returns the vendor-neutral tool declarations the agent exposes to the
// LLM. Parameters are JSON Schema objects; the adapter translates them into the
// SDK's tool/function-declaration format.
func toolDefs() []ports.ToolDef {
	return []ports.ToolDef{
		{
			Name:        toolVerifyCompany,
			Description: "Verifică o companie românească prin CUI/CIF folosind baza de date ANAF. Apelează imediat ce ai CUI-ul de la utilizator.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"cui": {
						"type": "string",
						"description": "Codul unic de identificare fiscală (CUI sau CIF) al companiei, doar cifre"
					}
				},
				"required": ["cui"]
			}`),
		},
		{
			Name:        toolSaveLead,
			Description: "Salvează lead-ul calificat după ce toate informațiile sunt colectate și compania verificată.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"product_name": {
						"type": "string",
						"description": "Produsul sau categoria de produse"
					},
					"quantity": {
						"type": "number",
						"description": "Cantitatea necesară"
					},
					"unit": {
						"type": "string",
						"description": "Unitatea de măsură (kg, buc, palet, camion, tonă etc.)"
					},
					"delivery_location": {
						"type": "string",
						"description": "Orașul sau județul pentru livrare"
					},
					"cui": {
						"type": "string",
						"description": "CUI-ul companiei verificate"
					},
					"company_name": {
						"type": "string",
						"description": "Numele companiei confirmat de ANAF (sau introdus de utilizator dacă ANAF indisponibil)"
					},
					"phone": {
						"type": "string",
						"description": "Numărul de telefon de contact al persoanei"
					},
					"email": {
						"type": "string",
						"description": "Adresa de email de contact al persoanei"
					}
				},
				"required": ["product_name", "quantity", "unit", "delivery_location", "cui", "company_name", "phone", "email"]
			}`),
		},
		{
			Name:        toolHandoff,
			Description: "Escaladează conversația către un coleg uman. Folosește când utilizatorul cere un om, este confuz sau contradictoriu, cererea este în afara scopului, sau nu poți continua în siguranță.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"reason": {
						"type": "string",
						"enum": ["user_request", "confusion_or_contradiction", "out_of_scope", "verification_failed", "unclassifiable_need", "other"],
						"description": "Motivul escaladării"
					},
					"summary": {
						"type": "string",
						"description": "Rezumat de un paragraf pentru coleg, în limba conversației"
					}
				},
				"required": ["reason"]
			}`),
		},
	}
}

// executeTool dispatches a tool call to its handler. Behavior is identical to the
// prior Gemini runner: handlers return a result map fed back to the LLM, and an
// error is surfaced to the caller as an {"error": ...} result.
func (c *Core) executeTool(ctx context.Context, conv *domain.Conversation, call ports.ToolCall) (map[string]any, error) {
	switch call.Name {
	case toolVerifyCompany:
		return c.toolVerifyCompany(ctx, conv, call.Args)
	case toolSaveLead:
		return c.toolSaveLead(ctx, conv, call.Args)
	case toolHandoff:
		return c.toolHandoff(ctx, conv, call.Args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func (c *Core) toolVerifyCompany(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	cui, _ := args["cui"].(string)
	cui = strings.TrimSpace(cui)

	_ = c.convRepo.UpdateState(ctx, conv.ID, domain.StateVerifying)

	company, err := c.verifier.Verify(ctx, cui)
	if err != nil {
		unavailable := strings.Contains(err.Error(), "unavailable")
		return map[string]any{
			"found":       false,
			"unavailable": unavailable,
			"reason":      err.Error(),
		}, nil
	}

	if !company.IsActive {
		return map[string]any{
			"found":  true,
			"active": false,
			"name":   company.Name,
			"reason": "Compania este inactivă în baza de date ANAF",
		}, nil
	}

	// Persist/update company record.
	_ = c.companyRepo.Upsert(ctx, company)

	// Update extracted fields.
	extracted := conv.Extracted
	if extracted == nil {
		extracted = make(map[string]any)
	}
	extracted["cui"] = cui
	extracted["company_name"] = company.Name
	_ = c.convRepo.UpdateExtracted(ctx, conv.ID, extracted)
	conv.Extracted = extracted

	return map[string]any{
		"found":   true,
		"active":  true,
		"name":    company.Name,
		"address": company.Address,
		"county":  company.County,
	}, nil
}

func (c *Core) toolSaveLead(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	cui, _ := args["cui"].(string)
	phone := strArg(args["phone"])
	email := strArg(args["email"])
	qty := extractFloat(args["quantity"])

	company, err := c.companyRepo.GetByCUI(ctx, cui)
	if err != nil {
		// ANAF was unavailable — upsert a minimal record from agent-collected args.
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
	}

	req := &domain.SourcingRequest{
		ProductName:      strArg(args["product_name"]),
		Quantity:         qty,
		Unit:             strArg(args["unit"]),
		DeliveryLocation: strArg(args["delivery_location"]),
	}

	// If a lead already exists for this conversation, update it in place.
	existingLead, err := c.leadRepo.GetByConversationID(ctx, conv.ID)
	if err == nil {
		contact := &domain.Contact{
			ID:        existingLead.ContactID,
			CompanyID: company.ID,
			Phone:     phone,
			Email:     email,
		}
		if err := c.contactRepo.Update(ctx, contact); err != nil {
			return nil, fmt.Errorf("update contact: %w", err)
		}
		if err := c.leadRepo.UpdateCompanyContact(ctx, existingLead.ID, company.ID, contact.ID); err != nil {
			return nil, fmt.Errorf("update lead: %w", err)
		}
		req.LeadID = existingLead.ID
		if err := c.sourcingRepo.UpdateByLeadID(ctx, req); err != nil {
			return nil, fmt.Errorf("update sourcing request: %w", err)
		}
		c.updateExtracted(ctx, conv, req, phone, email, args)
		c.notifyLeadSubmitted(ctx, conv, existingLead.ID, company, req, phone, email)
		return map[string]any{"saved": true, "lead_id": existingLead.ID, "updated": true}, nil
	}

	// No existing lead — create everything fresh.
	contact := &domain.Contact{
		CompanyID: company.ID,
		Phone:     phone,
		Email:     email,
	}
	if err := c.contactRepo.Create(ctx, contact); err != nil {
		return nil, fmt.Errorf("create contact: %w", err)
	}

	lead := &domain.Lead{
		ConversationID: conv.ID,
		CompanyID:      company.ID,
		ContactID:      contact.ID,
		Status:         "new",
	}
	if err := c.leadRepo.Create(ctx, lead); err != nil {
		return nil, fmt.Errorf("create lead: %w", err)
	}

	req.LeadID = lead.ID
	if err := c.sourcingRepo.Create(ctx, req); err != nil {
		return nil, fmt.Errorf("create sourcing request: %w", err)
	}

	c.updateExtracted(ctx, conv, req, phone, email, args)
	c.notifyLeadSubmitted(ctx, conv, lead.ID, company, req, phone, email)
	return map[string]any{"saved": true, "lead_id": lead.ID}, nil
}

func (c *Core) updateExtracted(ctx context.Context, conv *domain.Conversation, req *domain.SourcingRequest, phone, email string, args map[string]any) {
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

func strArg(v any) string {
	s, _ := v.(string)
	return s
}

func extractFloat(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case json.Number:
		f, err := n.Float64()
		if err == nil {
			return &f
		}
	}
	return nil
}
