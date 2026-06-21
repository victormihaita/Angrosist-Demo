// Package gemini is a thin adapter that satisfies the ports.LLM interface using
// Google's generative-ai-go SDK. It contains only SDK glue: it maps our neutral
// LLMRequest (system prompt, transcript, tool declarations) into genai calls and
// maps the genai response back into a neutral LLMResponse. It holds no repository
// access, no tool logic, and no conversation state — those live in the agent core.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"github.com/angrosist/demo/internal/ports"
)

// defaultModelName is the fallback used only when GEMINI_MODEL is unset. The
// model name is configuration, not a baked-in constant — set GEMINI_MODEL in the
// environment to change it without a code change (Hard Rule #1).
const defaultModelName = "gemini-2.5-flash"

// modelName resolves the Gemini model from the environment, falling back to a
// sane default for local/demo runs.
func modelName() string {
	if m := os.Getenv("GEMINI_MODEL"); m != "" {
		return m
	}
	return defaultModelName
}

// Adapter implements ports.LLM against the Gemini SDK.
type Adapter struct {
	clientOnce sync.Once
	client     *genai.Client
}

// New constructs a Gemini LLM adapter. The underlying genai client is created
// lazily on first use so wiring does not require network access.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) getClient(ctx context.Context) (*genai.Client, error) {
	var err error
	a.clientOnce.Do(func() {
		a.client, err = genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}
	if a.client == nil {
		return nil, errors.New("gemini: client not initialized")
	}
	return a.client, nil
}

// Complete maps the neutral request to a genai chat call and returns the
// assistant turn (text and/or tool calls).
func (a *Adapter) Complete(ctx context.Context, req ports.LLMRequest) (*ports.LLMResponse, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("gemini: empty message list")
	}

	client, err := a.getClient(ctx)
	if err != nil {
		return nil, err
	}

	model := client.GenerativeModel(modelName())
	model.SystemInstruction = &genai.Content{Parts: []genai.Part{genai.Text(req.System)}}
	model.Tools = toGenaiTools(req.Tools)

	cs := model.StartChat()

	// Everything but the final message becomes prior history; the final message
	// is sent as this turn's input.
	last := req.Messages[len(req.Messages)-1]
	cs.History = toGenaiHistory(req.Messages[:len(req.Messages)-1])

	parts, err := toGenaiParts(last)
	if err != nil {
		return nil, err
	}

	resp, err := cs.SendMessage(ctx, parts...)
	if err != nil {
		return nil, fmt.Errorf("gemini send: %w", err)
	}

	return fromGenaiResponse(resp)
}

// toGenaiTools maps neutral tool declarations (JSON-Schema parameters) into the
// genai function-declaration format.
func toGenaiTools(defs []ports.ToolDef) []*genai.Tool {
	if len(defs) == 0 {
		return nil
	}
	fns := make([]*genai.FunctionDeclaration, 0, len(defs))
	for _, d := range defs {
		schema, err := jsonSchemaToGenai(d.Parameters)
		if err != nil {
			continue
		}
		fns = append(fns, &genai.FunctionDeclaration{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  schema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: fns}}
}

// toGenaiHistory converts prior neutral messages into genai content history.
func toGenaiHistory(msgs []ports.LLMMessage) []*genai.Content {
	var history []*genai.Content
	for _, m := range msgs {
		parts, err := toGenaiParts(m)
		if err != nil || len(parts) == 0 {
			continue
		}
		history = append(history, &genai.Content{Role: genaiRole(m.Role), Parts: parts})
	}
	return history
}

// toGenaiParts converts a single neutral message into genai parts.
func toGenaiParts(m ports.LLMMessage) ([]genai.Part, error) {
	switch m.Role {
	case "tool":
		parts := make([]genai.Part, 0, len(m.ToolResults))
		for _, r := range m.ToolResults {
			parts = append(parts, genai.FunctionResponse{
				Name:     r.Name,
				Response: r.Content,
			})
		}
		return parts, nil

	case "assistant":
		var parts []genai.Part
		if m.Text != "" {
			parts = append(parts, genai.Text(m.Text))
		}
		for _, call := range m.ToolCalls {
			parts = append(parts, genai.FunctionCall{Name: call.Name, Args: call.Args})
		}
		return parts, nil

	default: // user
		if m.Text == "" {
			return nil, nil
		}
		return []genai.Part{genai.Text(m.Text)}, nil
	}
}

// genaiRole maps our neutral roles to genai's content roles. genai uses "model"
// for assistant turns; tool results are sent under the "user" role.
func genaiRole(role string) string {
	switch role {
	case "assistant":
		return "model"
	case "tool":
		return "user"
	default:
		return "user"
	}
}

// fromGenaiResponse maps a genai response into a neutral LLMResponse.
func fromGenaiResponse(resp *genai.GenerateContentResponse) (*ports.LLMResponse, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		return nil, errors.New("gemini: no candidates in response")
	}
	content := resp.Candidates[0].Content
	if content == nil {
		return nil, errors.New("gemini: nil content")
	}

	out := &ports.LLMResponse{}
	var text strings.Builder
	for _, part := range content.Parts {
		switch p := part.(type) {
		case genai.Text:
			text.WriteString(string(p))
		case genai.FunctionCall:
			out.ToolCalls = append(out.ToolCalls, ports.ToolCall{
				ID:   p.Name,
				Name: p.Name,
				Args: p.Args,
			})
		}
	}
	out.Text = text.String()
	return out, nil
}

// jsonSchema is the subset of JSON Schema the agent's tool declarations use.
type jsonSchema struct {
	Type        string                `json:"type"`
	Description string                `json:"description"`
	Properties  map[string]jsonSchema `json:"properties"`
	Required    []string              `json:"required"`
}

// jsonSchemaToGenai converts a JSON-Schema object into a genai.Schema.
func jsonSchemaToGenai(raw json.RawMessage) (*genai.Schema, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s jsonSchema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("gemini: parse tool schema: %w", err)
	}
	return convertSchema(s), nil
}

func convertSchema(s jsonSchema) *genai.Schema {
	out := &genai.Schema{
		Type:        genaiType(s.Type),
		Description: s.Description,
		Required:    s.Required,
	}
	if len(s.Properties) > 0 {
		out.Properties = make(map[string]*genai.Schema, len(s.Properties))
		for name, prop := range s.Properties {
			out.Properties[name] = convertSchema(prop)
		}
	}
	return out
}

func genaiType(t string) genai.Type {
	switch t {
	case "object":
		return genai.TypeObject
	case "string":
		return genai.TypeString
	case "number":
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	default:
		return genai.TypeUnspecified
	}
}
