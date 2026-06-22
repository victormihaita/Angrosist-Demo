package mock

import (
	"context"
	"testing"

	"github.com/angrosist/demo/internal/ports"
)

// TestComplete_VerifyThenSaveThenConfirm asserts the deterministic state machine:
// verify_company first (when nothing is verified), save_lead once a verify result
// is present, then a plain confirmation once the lead is saved.
func TestComplete_VerifyThenSaveThenConfirm(t *testing.T) {
	a := New()
	ctx := context.Background()

	// Round 1: a user message carrying a CUI -> verify_company with that CUI.
	r1, err := a.Complete(ctx, ports.LLMRequest{
		Messages: []ports.LLMMessage{{Role: "user", Text: "CUI RO12345678, produs: cafea"}},
	})
	if err != nil {
		t.Fatalf("round 1: %v", err)
	}
	if len(r1.ToolCalls) != 1 || r1.ToolCalls[0].Name != "verify_company" {
		t.Fatalf("round 1: want one verify_company call, got %+v", r1.ToolCalls)
	}
	if got := r1.ToolCalls[0].Args["cui"]; got != "12345678" {
		t.Fatalf("round 1: cui = %v; want 12345678", got)
	}

	// Round 2: with a verify result present -> save_lead populated from user text +
	// fallbacks.
	r2, err := a.Complete(ctx, ports.LLMRequest{
		Messages: []ports.LLMMessage{
			{Role: "user", Text: "CUI RO12345678, produs: cafea, email a@b.test"},
			{Role: "tool", ToolResults: []ports.ToolResult{{
				Name:    "verify_company",
				Content: map[string]any{"found": true, "name": "ACME SRL"},
			}}},
		},
	})
	if err != nil {
		t.Fatalf("round 2: %v", err)
	}
	if len(r2.ToolCalls) != 1 || r2.ToolCalls[0].Name != "save_lead" {
		t.Fatalf("round 2: want one save_lead call, got %+v", r2.ToolCalls)
	}
	args := r2.ToolCalls[0].Args
	if args["cui"] != "12345678" {
		t.Errorf("save_lead cui = %v; want 12345678", args["cui"])
	}
	if args["company_name"] != "ACME SRL" {
		t.Errorf("save_lead company_name = %v; want ACME SRL (from verify result)", args["company_name"])
	}
	if args["product_name"] != "cafea" {
		t.Errorf("save_lead product_name = %v; want cafea", args["product_name"])
	}
	if args["email"] != "a@b.test" {
		t.Errorf("save_lead email = %v; want a@b.test", args["email"])
	}
	// Unstated fields fall back to fixed test values.
	if args["unit"] != fallbackUnit {
		t.Errorf("save_lead unit = %v; want fallback %q", args["unit"], fallbackUnit)
	}

	// Round 3: with a successful save_lead result present -> plain confirmation text.
	r3, err := a.Complete(ctx, ports.LLMRequest{
		Messages: []ports.LLMMessage{
			{Role: "tool", ToolResults: []ports.ToolResult{{
				Name:    "verify_company",
				Content: map[string]any{"found": true},
			}}},
			{Role: "tool", ToolResults: []ports.ToolResult{{
				Name:    "save_lead",
				Content: map[string]any{"saved": true, "lead_id": "abc"},
			}}},
		},
	})
	if err != nil {
		t.Fatalf("round 3: %v", err)
	}
	if len(r3.ToolCalls) != 0 {
		t.Fatalf("round 3: want no tool calls, got %+v", r3.ToolCalls)
	}
	if r3.Text == "" {
		t.Fatalf("round 3: want confirmation text, got empty")
	}
}

// TestComplete_NoCUIUsesFallback asserts that without any CUI in the transcript the
// adapter still drives the flow using the fixed test CUI.
func TestComplete_NoCUIUsesFallback(t *testing.T) {
	a := New()
	r, err := a.Complete(context.Background(), ports.LLMRequest{
		Messages: []ports.LLMMessage{{Role: "user", Text: "vreau sa cumpar ceva"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Args["cui"] != fallbackCUI {
		t.Fatalf("want verify_company with fallback cui %q, got %+v", fallbackCUI, r.ToolCalls)
	}
}

// ensure the adapter satisfies the port.
var _ ports.LLM = (*Adapter)(nil)
