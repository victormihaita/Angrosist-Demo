// Package claude is a thin adapter that satisfies the ports.LLM interface using
// Anthropic's official Go SDK. It contains only SDK glue: it maps our neutral
// LLMRequest (system prompt, transcript, tool declarations) into Anthropic
// Messages API params and maps the response back into a neutral LLMResponse. It
// holds no repository access, no tool logic, and no conversation state — those
// live in the agent core.
//
// This adapter exists to prove the LLM provider is swappable: it implements the
// exact same ports.LLM seam as the Gemini demo adapter, so selecting Claude is a
// configuration change, not a domain change.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/angrosist/demo/internal/ports"
)

// Configuration defaults. These are fallbacks used only when the corresponding
// environment variable is unset; the real values are configuration, never baked
// into the binary as decisions (Hard Rule #1). The default model is Haiku, the
// recommended high-volume qualification model per docs/specs/AI_AGENT_SPEC.md §9.
const (
	defaultModel     = "claude-haiku-4-5"
	defaultMaxTokens = int64(1024)
)

// config holds the resolved, env-driven knobs for a Claude call.
type config struct {
	model     string
	maxTokens int64
}

// loadConfig reads the Claude knobs from the environment, applying sane defaults
// for any that are unset. The API key is read by the SDK itself from
// ANTHROPIC_API_KEY and is never handled here.
func loadConfig() config {
	c := config{model: defaultModel, maxTokens: defaultMaxTokens}
	if m := os.Getenv("CLAUDE_MODEL"); m != "" {
		c.model = m
	}
	if t := os.Getenv("CLAUDE_MAX_TOKENS"); t != "" {
		if n, err := strconv.ParseInt(t, 10, 64); err == nil && n > 0 {
			c.maxTokens = n
		}
	}
	return c
}

// Adapter implements ports.LLM against the Anthropic Messages API.
type Adapter struct {
	cfg config

	clientOnce sync.Once
	client     anthropic.Client
}

// New constructs a Claude LLM adapter. The underlying Anthropic client is built
// lazily on first use so wiring does not require an API key to be present at
// startup (the Gemini default path keeps the demo working without one).
func New() *Adapter {
	return &Adapter{cfg: loadConfig()}
}

func (a *Adapter) getClient() anthropic.Client {
	a.clientOnce.Do(func() {
		// The SDK reads ANTHROPIC_API_KEY from the environment by default. We
		// pass it explicitly when present so config resolution stays in one place
		// and the key never appears anywhere but this seam.
		if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			a.client = anthropic.NewClient(option.WithAPIKey(key))
		} else {
			a.client = anthropic.NewClient()
		}
	})
	return a.client
}

// Complete maps the neutral request to an Anthropic Messages call and returns the
// assistant turn (text and/or tool calls).
func (a *Adapter) Complete(ctx context.Context, req ports.LLMRequest) (*ports.LLMResponse, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("claude: empty message list")
	}

	params, err := buildParams(a.cfg, req)
	if err != nil {
		return nil, err
	}

	client := a.getClient()
	resp, err := client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claude send: %w", err)
	}

	return fromMessage(resp)
}

// buildParams translates a neutral LLMRequest into Anthropic MessageNewParams. It
// is pure (no network, no client) so it can be unit-tested in isolation.
func buildParams(cfg config, req ports.LLMRequest) (anthropic.MessageNewParams, error) {
	msgs, err := toMessageParams(req.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(cfg.model),
		MaxTokens: cfg.maxTokens,
		Messages:  msgs,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	if tools := toToolUnions(req.Tools); len(tools) > 0 {
		params.Tools = tools
	}
	return params, nil
}

// toMessageParams converts the neutral transcript into Anthropic message params,
// mapping our user/assistant/tool roles onto Anthropic's user/assistant turns.
// Tool results are carried on a user-role message of tool_result blocks, per the
// Messages API.
func toMessageParams(msgs []ports.LLMMessage) ([]anthropic.MessageParam, error) {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.ToolCalls)+1)
			if m.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Text))
			}
			for _, call := range m.ToolCalls {
				input, err := toolInput(call.Args)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(call.ID, input, call.Name))
			}
			if len(blocks) == 0 {
				continue
			}
			out = append(out, anthropic.NewAssistantMessage(blocks...))

		case "tool":
			blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.ToolResults))
			for _, r := range m.ToolResults {
				content, err := toolResultContent(r.Content)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, anthropic.NewToolResultBlock(r.ID, content, false))
			}
			if len(blocks) == 0 {
				continue
			}
			out = append(out, anthropic.NewUserMessage(blocks...))

		default: // user
			if m.Text == "" {
				continue
			}
			out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Text)))
		}
	}
	return out, nil
}

// toolInput shapes a neutral tool-call argument map into the SDK's expected
// input value. A nil map becomes an empty object so the wire payload is valid.
func toolInput(args map[string]any) (any, error) {
	if args == nil {
		return map[string]any{}, nil
	}
	return args, nil
}

// toolResultContent serializes a neutral tool-result map into the single JSON
// string the Messages API tool_result block expects.
func toolResultContent(content map[string]any) (string, error) {
	if content == nil {
		return "{}", nil
	}
	b, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("claude: marshal tool result: %w", err)
	}
	return string(b), nil
}

// toToolUnions maps neutral tool declarations (JSON-Schema parameters) into the
// Anthropic tool format. Each declaration's JSON Schema is parsed to extract the
// "properties" and "required" members that ToolInputSchemaParam expects. A tool
// whose schema fails to parse is skipped rather than failing the whole turn.
func toToolUnions(defs []ports.ToolDef) []anthropic.ToolUnionParam {
	if len(defs) == 0 {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		props, required, err := schemaToInput(d.Parameters)
		if err != nil {
			continue
		}
		tool := anthropic.ToolParam{
			Name: d.Name,
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: props,
				Required:   required,
			},
		}
		if d.Description != "" {
			tool.Description = anthropic.String(d.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return out
}

// schemaToInput extracts the "properties" object and "required" list from a JSON
// Schema object (our ToolDef.Parameters). It is pure and the primary unit of the
// tool-schema conversion test.
func schemaToInput(raw json.RawMessage) (properties any, required []string, err error) {
	if len(raw) == 0 {
		return map[string]any{}, nil, nil
	}
	var schema struct {
		Properties json.RawMessage `json:"properties"`
		Required   []string        `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, nil, fmt.Errorf("claude: parse tool schema: %w", err)
	}
	if len(schema.Properties) == 0 {
		return map[string]any{}, schema.Required, nil
	}
	var props map[string]any
	if err := json.Unmarshal(schema.Properties, &props); err != nil {
		return nil, nil, fmt.Errorf("claude: parse tool schema properties: %w", err)
	}
	return props, schema.Required, nil
}

// fromMessage maps an Anthropic response Message into a neutral LLMResponse,
// collecting assistant text and any tool_use blocks. It is pure so it can be
// unit-tested against a constructed Message.
func fromMessage(msg *anthropic.Message) (*ports.LLMResponse, error) {
	if msg == nil {
		return nil, errors.New("claude: nil response")
	}

	out := &ports.LLMResponse{}
	var text strings.Builder
	for _, block := range msg.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			text.WriteString(v.Text)
		case anthropic.ToolUseBlock:
			args, err := parseToolArgs(v.Input)
			if err != nil {
				return nil, err
			}
			out.ToolCalls = append(out.ToolCalls, ports.ToolCall{
				ID:   block.ID,
				Name: v.Name,
				Args: args,
			})
		}
	}
	out.Text = text.String()
	return out, nil
}

// parseToolArgs decodes a tool_use block's raw JSON input into a neutral argument
// map. Empty input yields an empty map rather than nil.
func parseToolArgs(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	args := map[string]any{}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("claude: parse tool args: %w", err)
	}
	return args, nil
}
