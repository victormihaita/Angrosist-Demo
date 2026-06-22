// Package mock is a deterministic, request-driven LLM adapter used for TESTS and
// LOCAL/OFFLINE demos ONLY. It is NOT a production provider and must never be the
// default: it is selected explicitly via LLM_PROVIDER=mock so the assembled stack
// can be exercised end-to-end without calling a real language model.
//
// It satisfies the same ports.LLM seam as the Gemini (demo) and Claude (prod)
// adapters, proving the provider is swappable with zero changes to the agent core
// (Hard Rule #2). It holds no repository access, no keys, and no conversation
// state — it only inspects the neutral LLMRequest it is handed and emits the next
// deterministic step of the Angrosist buyer happy path.
//
// Behavior (request-driven, so it drives the flow to completion without scripted
// back-and-forth):
//
//   - If no company has been verified yet (no verify_company tool RESULT appears in
//     the transcript), it returns a verify_company tool call, with a CUI parsed from
//     the user's messages (or a fixed test CUI when none is present).
//   - Once a verify_company tool result is present but no save_lead has succeeded, it
//     returns a save_lead tool call populated from a deterministic mapping of the
//     fields collected from the user messages (product/quantity/unit/location/cui/
//     company_name/phone/email), falling back to fixed test values for anything not
//     stated. This also covers the one-shot case where the first user message already
//     contains everything: verify happens first, then save_lead on the next round.
//   - After save_lead has succeeded, it returns a plain confirmation text and no tool
//     calls, ending the turn.
//
// Exhaustive multi-vertical scripting is intentionally out of scope: the Angrosist
// buyer happy path is the target (the E2E critical-path test).
package mock

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/angrosist/demo/internal/ports"
)

// Tool names mirrored from the agent core. They are duplicated here (rather than
// imported) so this test adapter stays a leaf with no dependency on the agent
// package, exactly like the real LLM adapters.
const (
	toolVerifyCompany = "verify_company"
	toolSaveLead      = "save_lead"
)

// Deterministic fallbacks used when a field is not stated in the conversation.
// These are obvious TEST values, never secrets or real data (Hard Rule #1). The
// CUI is a valid positive integer so the DemoANAF demo-mode verifier accepts it.
const (
	fallbackCUI         = "12345678"
	fallbackCompanyName = "Demo Company SRL"
	fallbackProduct     = "cafea boabe"
	fallbackQuantity    = 500.0
	fallbackUnit        = "kg"
	fallbackLocation    = "Cluj-Napoca"
	fallbackPhone       = "+40700000000"
	fallbackEmail       = "buyer@example.test"

	confirmationText = "Mulțumesc! Am salvat cererea ta și un coleg te va contacta în curând."
)

// cuiPattern matches a Romanian CUI (an optional RO prefix + 2..10 digits) inside
// free text, so a one-shot user message like "CUI RO12345678" yields the digits.
var cuiPattern = regexp.MustCompile(`(?i)\bRO\s*([0-9]{2,10})\b|\b([0-9]{6,10})\b`)

// emailPattern is a deliberately permissive email matcher for pulling a contact
// email out of free text; the agent/domain re-validate downstream.
var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// phonePattern matches a Romanian-style phone (optional +, 9..15 digits with
// spaces) so a stated number is preferred over the fallback.
var phonePattern = regexp.MustCompile(`\+?[0-9][0-9 ]{8,15}[0-9]`)

// Adapter implements ports.LLM with a deterministic, offline state machine.
type Adapter struct{}

// New constructs the mock LLM adapter. It needs no configuration: it is fully
// deterministic and makes no network calls.
func New() *Adapter { return &Adapter{} }

// Complete returns the next deterministic step of the Angrosist buyer happy path,
// derived purely from the transcript in req. See the package doc for the state
// machine. It never errors for a well-formed request.
func (a *Adapter) Complete(_ context.Context, req ports.LLMRequest) (*ports.LLMResponse, error) {
	state := scan(req.Messages)

	switch {
	case state.leadSaved:
		// The lead is persisted — close the turn with a plain confirmation.
		return &ports.LLMResponse{Text: confirmationText}, nil

	case !state.companyVerified:
		// No company verified yet — ask our code to verify the CUI we found (or a
		// fixed test CUI). The agent core executes verify_company against the port.
		cui := state.cui
		if cui == "" {
			cui = fallbackCUI
		}
		return &ports.LLMResponse{
			ToolCalls: []ports.ToolCall{{
				ID:   toolVerifyCompany,
				Name: toolVerifyCompany,
				Args: map[string]any{"cui": cui},
			}},
		}, nil

	default:
		// Company verified, lead not yet saved — submit the collected fields.
		return &ports.LLMResponse{
			ToolCalls: []ports.ToolCall{{
				ID:   toolSaveLead,
				Name: toolSaveLead,
				Args: state.saveLeadArgs(),
			}},
		}, nil
	}
}

// convoState is the deterministic view the adapter derives from the transcript:
// whether verify_company / save_lead have already happened, and the best-known
// field values pulled from the user messages.
type convoState struct {
	companyVerified bool
	leadSaved       bool

	cui         string
	companyName string
	product     string
	quantity    *float64
	unit        string
	location    string
	phone       string
	email       string
}

// scan walks the transcript once, deriving tool progress (from tool RESULTS) and
// the field values stated by the user (from user text). Tool progress is read from
// results, never from the adapter's own prior calls, so it is robust to history
// rebuilt by the agent core across turns.
func scan(msgs []ports.LLMMessage) convoState {
	var s convoState
	var userText strings.Builder

	for _, m := range msgs {
		switch m.Role {
		case "user":
			userText.WriteString(" ")
			userText.WriteString(m.Text)
		case "tool":
			for _, r := range m.ToolResults {
				switch r.Name {
				case toolVerifyCompany:
					if found, _ := r.Content["found"].(bool); found {
						s.companyVerified = true
					}
					if name, _ := r.Content["name"].(string); name != "" {
						s.companyName = name
					}
				case toolSaveLead:
					if saved, _ := r.Content["saved"].(bool); saved {
						s.leadSaved = true
					}
				}
			}
		}
	}

	parseUserFields(strings.TrimSpace(userText.String()), &s)
	return s
}

// parseUserFields extracts the deterministic field values from the concatenated
// user text. Anything not found is left empty so saveLeadArgs falls back to the
// fixed test values. Field labels are matched loosely (RO + EN) so a one-shot
// message can populate everything.
func parseUserFields(text string, s *convoState) {
	if text == "" {
		return
	}

	if m := cuiPattern.FindStringSubmatch(text); m != nil {
		if m[1] != "" {
			s.cui = m[1]
		} else if m[2] != "" {
			s.cui = m[2]
		}
	}
	if m := emailPattern.FindString(text); m != "" {
		s.email = m
	}
	if m := phonePattern.FindString(text); m != "" {
		s.phone = strings.ReplaceAll(m, " ", "")
	}

	s.product = labeledValue(text, "produs", "product")
	s.unit = labeledValue(text, "unitate", "unit")
	s.location = labeledValue(text, "locatie", "locație", "location", "livrare", "oras", "oraș")
	if s.companyName == "" {
		s.companyName = labeledValue(text, "companie", "company", "firma", "firmă")
	}
	if q := labeledNumber(text, "cantitate", "quantity", "qty"); q != nil {
		s.quantity = q
	}
}

// saveLeadArgs builds the deterministic save_lead arguments, preferring values
// stated by the user and falling back to the fixed test values for the rest. The
// company name prefers the ANAF-confirmed name captured from the verify result.
func (s convoState) saveLeadArgs() map[string]any {
	cui := s.cui
	if cui == "" {
		cui = fallbackCUI
	}
	companyName := s.companyName
	if companyName == "" {
		companyName = fallbackCompanyName
	}
	product := s.product
	if product == "" {
		product = fallbackProduct
	}
	quantity := fallbackQuantity
	if s.quantity != nil {
		quantity = *s.quantity
	}
	unit := s.unit
	if unit == "" {
		unit = fallbackUnit
	}
	location := s.location
	if location == "" {
		location = fallbackLocation
	}
	phone := s.phone
	if phone == "" {
		phone = fallbackPhone
	}
	email := s.email
	if email == "" {
		email = fallbackEmail
	}

	return map[string]any{
		"product_name":      product,
		"quantity":          quantity,
		"unit":              unit,
		"delivery_location": location,
		"cui":               cui,
		"company_name":      companyName,
		"phone":             phone,
		"email":             email,
	}
}

// labeledValue returns the text following the first matching "label: value" or
// "label value" occurrence (case-insensitive), trimmed at the next separator.
// Empty when no label matches.
func labeledValue(text string, labels ...string) string {
	lower := strings.ToLower(text)
	for _, label := range labels {
		idx := strings.Index(lower, label)
		if idx < 0 {
			continue
		}
		rest := text[idx+len(label):]
		rest = strings.TrimLeft(rest, " :=-\t")
		// Cut at the next clause/sentence boundary so we capture one value only.
		if cut := strings.IndexAny(rest, ",.;\n"); cut >= 0 {
			rest = rest[:cut]
		}
		if v := strings.TrimSpace(rest); v != "" {
			return v
		}
	}
	return ""
}

// labeledNumber parses the first number following a matching label. Returns nil
// when no labeled number is present.
func labeledNumber(text string, labels ...string) *float64 {
	v := labeledValue(text, labels...)
	if v == "" {
		return nil
	}
	// Take the leading numeric token (e.g. "500 kg" -> 500).
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return nil
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(fields[0], ",", "."), 64)
	if err != nil {
		return nil
	}
	return &n
}
