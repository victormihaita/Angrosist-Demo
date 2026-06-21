package ports

import (
	"context"
	"encoding/json"
)

// LLM is the vendor-neutral seam to a language model. Claude (production) and
// Gemini (demo) are two adapters behind this single interface. Adapters map our
// neutral request/response shapes to and from their SDK's wire format; they hold
// no repository access, no tool logic, and no conversation state.
//
// The model returns either assistant text or tool calls. The agent core — never
// the adapter — executes the requested tools and feeds the results back as a
// follow-up Complete call.
type LLM interface {
	// Complete runs one model call and returns the assistant turn (text and/or
	// tool calls). Tool results are passed back via subsequent Complete calls
	// carrying LLMMessage entries with role "tool".
	Complete(ctx context.Context, req LLMRequest) (*LLMResponse, error)
}

// LLMRequest is a single, self-contained model invocation: a system prompt, the
// full normalized transcript so far, and the tools the model may call.
type LLMRequest struct {
	// System is the system prompt (per vertical + language).
	System string
	// Messages is the running transcript in chronological order.
	Messages []LLMMessage
	// Tools are the tool declarations the model may invoke.
	Tools []ToolDef
}

// LLMMessage is one normalized transcript entry.
//
// Role semantics:
//   - "user": Text holds the user's message.
//   - "assistant": Text and/or ToolCalls hold the model's output.
//   - "tool": ToolResults holds the outcome of previously requested tool calls.
type LLMMessage struct {
	Role        string
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
}

// ToolDef is a vendor-neutral tool declaration. Parameters is a JSON Schema
// object describing the tool's arguments.
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolCall is a model-issued request to invoke a tool.
type ToolCall struct {
	// ID correlates a call with its result. Some providers (e.g. Gemini) key
	// results by tool name rather than an opaque id; adapters may use Name as
	// the id in that case.
	ID   string
	Name string
	Args map[string]any
}

// ToolResult is the outcome of executing a ToolCall, fed back to the model.
type ToolResult struct {
	ID      string
	Name    string
	Content map[string]any
}

// LLMResponse is the assistant's turn: free text and/or tool calls to execute.
type LLMResponse struct {
	Text      string
	ToolCalls []ToolCall
}
