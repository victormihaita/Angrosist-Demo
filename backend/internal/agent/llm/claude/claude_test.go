package claude

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/angrosist/demo/internal/ports"
)

// TestLoadConfigDefaults verifies the env-driven config falls back to the
// documented defaults when nothing is set, and that no model name is hardcoded
// anywhere but the explicit default constant.
func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("CLAUDE_MODEL", "")
	t.Setenv("CLAUDE_MAX_TOKENS", "")

	cfg := loadConfig()
	if cfg.model != defaultModel {
		t.Fatalf("model = %q, want %q", cfg.model, defaultModel)
	}
	if cfg.maxTokens != defaultMaxTokens {
		t.Fatalf("maxTokens = %d, want %d", cfg.maxTokens, defaultMaxTokens)
	}
}

// TestLoadConfigEnvOverride verifies env vars override the defaults and that a
// bad CLAUDE_MAX_TOKENS value is ignored in favor of the default.
func TestLoadConfigEnvOverride(t *testing.T) {
	t.Setenv("CLAUDE_MODEL", "claude-opus-4-8")
	t.Setenv("CLAUDE_MAX_TOKENS", "2048")
	cfg := loadConfig()
	if cfg.model != "claude-opus-4-8" {
		t.Fatalf("model = %q, want claude-opus-4-8", cfg.model)
	}
	if cfg.maxTokens != 2048 {
		t.Fatalf("maxTokens = %d, want 2048", cfg.maxTokens)
	}

	t.Setenv("CLAUDE_MAX_TOKENS", "not-a-number")
	if got := loadConfig().maxTokens; got != defaultMaxTokens {
		t.Fatalf("invalid CLAUDE_MAX_TOKENS: maxTokens = %d, want default %d", got, defaultMaxTokens)
	}
}

// TestSchemaToInput checks the JSON-Schema → (properties, required) conversion
// that feeds Anthropic's ToolInputSchemaParam.
func TestSchemaToInput(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "object",
		"properties": {
			"cui": {"type": "string", "description": "Romanian tax id"},
			"quantity": {"type": "integer"}
		},
		"required": ["cui"]
	}`)

	props, required, err := schemaToInput(raw)
	if err != nil {
		t.Fatalf("schemaToInput: %v", err)
	}

	pm, ok := props.(map[string]any)
	if !ok {
		t.Fatalf("properties is %T, want map[string]any", props)
	}
	if _, ok := pm["cui"]; !ok {
		t.Fatalf("properties missing cui: %v", pm)
	}
	if _, ok := pm["quantity"]; !ok {
		t.Fatalf("properties missing quantity: %v", pm)
	}
	if !reflect.DeepEqual(required, []string{"cui"}) {
		t.Fatalf("required = %v, want [cui]", required)
	}
}

// TestSchemaToInputEmpty checks that an empty/absent schema yields an empty
// properties object rather than an error or nil.
func TestSchemaToInputEmpty(t *testing.T) {
	props, required, err := schemaToInput(nil)
	if err != nil {
		t.Fatalf("schemaToInput(nil): %v", err)
	}
	if pm, ok := props.(map[string]any); !ok || len(pm) != 0 {
		t.Fatalf("properties = %v, want empty map", props)
	}
	if required != nil {
		t.Fatalf("required = %v, want nil", required)
	}
}

// TestToToolUnions verifies a neutral ToolDef becomes a populated Anthropic tool.
func TestToToolUnions(t *testing.T) {
	defs := []ports.ToolDef{{
		Name:        "verify_company",
		Description: "Verify a Romanian company by CUI.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"cui":{"type":"string"}},"required":["cui"]}`),
	}}

	tools := toToolUnions(defs)
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	tool := tools[0].OfTool
	if tool == nil {
		t.Fatal("OfTool is nil")
	}
	if tool.Name != "verify_company" {
		t.Fatalf("name = %q, want verify_company", tool.Name)
	}
	if tool.Description.Value != "Verify a Romanian company by CUI." {
		t.Fatalf("description = %q", tool.Description.Value)
	}
	if !reflect.DeepEqual(tool.InputSchema.Required, []string{"cui"}) {
		t.Fatalf("required = %v, want [cui]", tool.InputSchema.Required)
	}
}

// TestBuildParams checks the full LLMRequest → MessageNewParams mapping for the
// scalar config and system prompt, plus that tools and messages flow through.
func TestBuildParams(t *testing.T) {
	cfg := config{model: "claude-haiku-4-5", maxTokens: 512}
	req := ports.LLMRequest{
		System: "You are a qualifier.",
		Messages: []ports.LLMMessage{
			{Role: "user", Text: "vreau lapte"},
		},
		Tools: []ports.ToolDef{{
			Name:       "submit_lead",
			Parameters: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		}},
	}

	params, err := buildParams(cfg, req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if params.Model != anthropic.Model("claude-haiku-4-5") {
		t.Fatalf("model = %q", params.Model)
	}
	if params.MaxTokens != 512 {
		t.Fatalf("maxTokens = %d, want 512", params.MaxTokens)
	}
	if len(params.System) != 1 || params.System[0].Text != "You are a qualifier." {
		t.Fatalf("system = %#v", params.System)
	}
	if len(params.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(params.Messages))
	}
	if len(params.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(params.Tools))
	}
}

// TestToMessageParamsRoles checks the user/assistant/tool history mapping,
// including a tool-call turn followed by a tool-result turn.
func TestToMessageParamsRoles(t *testing.T) {
	msgs := []ports.LLMMessage{
		{Role: "user", Text: "verifica 12345"},
		{Role: "assistant", Text: "checking", ToolCalls: []ports.ToolCall{
			{ID: "toolu_1", Name: "verify_company", Args: map[string]any{"cui": "12345"}},
		}},
		{Role: "tool", ToolResults: []ports.ToolResult{
			{ID: "toolu_1", Name: "verify_company", Content: map[string]any{"valid": true}},
		}},
		{Role: "assistant", Text: "ok verified"},
	}

	out, err := toMessageParams(msgs)
	if err != nil {
		t.Fatalf("toMessageParams: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("got %d messages, want 4", len(out))
	}

	// Marshal the assistant tool-call turn and assert the tool_use block carries
	// the id, name, and input we provided.
	b, err := json.Marshal(out[1])
	if err != nil {
		t.Fatalf("marshal assistant turn: %v", err)
	}
	s := string(b)
	for _, want := range []string{"\"tool_use\"", "toolu_1", "verify_company", "\"12345\""} {
		if !strings.Contains(s, want) {
			t.Fatalf("assistant tool_use turn missing %q in %s", want, s)
		}
	}

	// The tool-result turn must be a user-role tool_result referencing the id.
	rb, err := json.Marshal(out[2])
	if err != nil {
		t.Fatalf("marshal tool result turn: %v", err)
	}
	rs := string(rb)
	for _, want := range []string{"\"tool_result\"", "toolu_1", "\"role\":\"user\""} {
		if !strings.Contains(rs, want) {
			t.Fatalf("tool_result turn missing %q in %s", want, rs)
		}
	}
}

// TestToMessageParamsSkipsEmpty ensures empty user/assistant turns are dropped so
// we never send a blank turn to the API.
func TestToMessageParamsSkipsEmpty(t *testing.T) {
	out, err := toMessageParams([]ports.LLMMessage{
		{Role: "user", Text: ""},
		{Role: "assistant"},
		{Role: "user", Text: "hi"},
	})
	if err != nil {
		t.Fatalf("toMessageParams: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d messages, want 1", len(out))
	}
}

// TestFromMessage parses a constructed Anthropic Message (built from wire JSON)
// into a neutral LLMResponse with both text and a tool call.
func TestFromMessage(t *testing.T) {
	wire := `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-haiku-4-5",
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "Let me verify that."},
			{"type": "tool_use", "id": "toolu_99", "name": "verify_company", "input": {"cui": "RO123"}}
		]
	}`

	var msg anthropic.Message
	if err := json.Unmarshal([]byte(wire), &msg); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}

	resp, err := fromMessage(&msg)
	if err != nil {
		t.Fatalf("fromMessage: %v", err)
	}
	if resp.Text != "Let me verify that." {
		t.Fatalf("text = %q", resp.Text)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_99" || tc.Name != "verify_company" {
		t.Fatalf("tool call id/name = %q/%q", tc.ID, tc.Name)
	}
	if got, _ := tc.Args["cui"].(string); got != "RO123" {
		t.Fatalf("tool call args[cui] = %v, want RO123", tc.Args["cui"])
	}
}

// TestFromMessageTextOnly checks the plain assistant-text path.
func TestFromMessageTextOnly(t *testing.T) {
	wire := `{
		"id": "msg_2",
		"type": "message",
		"role": "assistant",
		"model": "claude-haiku-4-5",
		"stop_reason": "end_turn",
		"content": [{"type": "text", "text": "Ce produs cauti?"}]
	}`
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(wire), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resp, err := fromMessage(&msg)
	if err != nil {
		t.Fatalf("fromMessage: %v", err)
	}
	if resp.Text != "Ce produs cauti?" {
		t.Fatalf("text = %q", resp.Text)
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("got %d tool calls, want 0", len(resp.ToolCalls))
	}
}
