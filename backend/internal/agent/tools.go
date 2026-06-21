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

// verifyCompanyToolDef and handoffToolDef are shared across all flows (every RO
// company has a CUI, and any flow can escalate). They are functions so each flow's
// ToolDefs can compose them with its own save_lead schema.
func verifyCompanyToolDef() ports.ToolDef {
	return ports.ToolDef{
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
	}
}

func handoffToolDef() ports.ToolDef {
	return ports.ToolDef{
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
	}
}

// angrosistBuyerToolDefs is the EXACT tool set the prior single-vertical runner
// exposed: verify_company, save_lead (Angrosist buyer schema), handoff_to_human,
// in that order. Preserving the order keeps the existing flow's behavior and tests
// identical.
func angrosistBuyerToolDefs() []ports.ToolDef {
	return []ports.ToolDef{
		verifyCompanyToolDef(),
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
		handoffToolDef(),
	}
}

// pcBuyerToolDefs is the PalletClearance buyer tool set: verify_company (CUI
// opportunistic), save_lead (buyer-feed schema), handoff_to_human.
func pcBuyerToolDefs() []ports.ToolDef {
	return []ports.ToolDef{
		verifyCompanyToolDef(),
		{
			Name:        toolSaveLead,
			Description: "Salvează profilul de cumpărător PalletClearance după ce ai colectat preferințele și datele de contact.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"categories": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Categoriile de produse care îl interesează"
					},
					"volume": {
						"type": "string",
						"description": "Capacitatea de volum/manipulare (ex. 2 camioane/lună)"
					},
					"countries": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Țările sursă din care ar cumpăra (coduri ISO sau nume)"
					},
					"near_expiry_ok": {
						"type": "boolean",
						"description": "Acceptă stoc cu termen scurt / aproape de expirare"
					},
					"subscribe": {
						"type": "boolean",
						"description": "Vrea să fie abonat la feed-ul de loturi"
					},
					"cui": {
						"type": "string",
						"description": "CUI-ul companiei (opțional)"
					},
					"company_name": {
						"type": "string",
						"description": "Numele companiei (opțional)"
					},
					"phone": {
						"type": "string",
						"description": "Numărul de telefon de contact"
					},
					"email": {
						"type": "string",
						"description": "Adresa de email de contact"
					}
				},
				"required": ["categories", "volume", "countries", "near_expiry_ok", "subscribe", "phone", "email"]
			}`),
		},
		handoffToolDef(),
	}
}

// pcSellerToolDefs is the PalletClearance seller tool set: verify_company,
// save_lead (lot schema), handoff_to_human. (The blocking photo gate / upload_media
// land in part A2; save_lead is a normal submit here.)
func pcSellerToolDefs() []ports.ToolDef {
	return []ports.ToolDef{
		verifyCompanyToolDef(),
		{
			Name:        toolSaveLead,
			Description: "Salvează listarea lotului PalletClearance după ce ai colectat detaliile lotului, compania și datele de contact.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"stock_type": {
						"type": "string",
						"description": "Tipul de stoc (overstock / lichidare / retururi / termen scurt)"
					},
					"category": {
						"type": "string",
						"description": "Categoria de produse a lotului"
					},
					"quantity": {
						"type": "number",
						"description": "Cantitatea"
					},
					"unit": {
						"type": "string",
						"description": "Unitatea de măsură (paleți / cutii / tone)"
					},
					"location": {
						"type": "string",
						"description": "Unde se află stocul"
					},
					"country": {
						"type": "string",
						"description": "Țara unde se află stocul (cod ISO sau nume)"
					},
					"expiry": {
						"type": "string",
						"description": "Data de expirare / termenul (format ISO YYYY-MM-DD dacă e posibil)"
					},
					"target_price": {
						"type": "number",
						"description": "Prețul cerut (opțional)"
					},
					"confidential": {
						"type": "boolean",
						"description": "Listarea este confidențială (ascunde identitatea vânzătorului)"
					},
					"cui": {
						"type": "string",
						"description": "CUI-ul companiei vânzătoare"
					},
					"company_name": {
						"type": "string",
						"description": "Numele companiei confirmat de ANAF (sau introdus de utilizator dacă ANAF indisponibil)"
					},
					"phone": {
						"type": "string",
						"description": "Numărul de telefon de contact"
					},
					"email": {
						"type": "string",
						"description": "Adresa de email de contact"
					}
				},
				"required": ["stock_type", "category", "quantity", "location", "expiry", "confidential", "cui", "company_name", "phone", "email"]
			}`),
		},
		handoffToolDef(),
	}
}

// executeTool dispatches a tool call to its handler. verify_company and
// handoff_to_human are flow-agnostic; save_lead routes to the active flow's
// typed-request writer (sourcing_request | listing | buyer_profile).
func (c *Core) executeTool(ctx context.Context, conv *domain.Conversation, flow *Flow, call ports.ToolCall) (map[string]any, error) {
	switch call.Name {
	case toolVerifyCompany:
		return c.toolVerifyCompany(ctx, conv, call.Args)
	case toolSaveLead:
		return flow.Submit(ctx, c, conv, call.Args)
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

func strArg(v any) string {
	s, _ := v.(string)
	return s
}

func boolArg(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		s := strings.ToLower(strings.TrimSpace(b))
		return s == "true" || s == "yes" || s == "da" || s == "1"
	}
	return false
}

// strSliceArg coerces a tool arg into a []string. The LLM may send a JSON array
// (decoded to []any of strings) or a single comma-separated string; both are
// normalized so the flow gets a clean slice.
func strSliceArg(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case []string:
		return t
	case string:
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return nil
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
