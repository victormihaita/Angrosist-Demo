// Package agent is the vendor-neutral, channel-agnostic agent core. It owns the
// conversational turn loop, the qualification state machine, and the tool
// registry. It depends only on the LLM port and the repository/service ports —
// never on a language-model SDK. Swapping Claude for Gemini (or any future
// provider) is a wiring change in the composition root, not a change here.
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// maxToolRounds bounds the tool-call loop within a single turn, preventing an
// unbounded back-and-forth if the model keeps requesting tools.
const maxToolRounds = 8

// Core executes conversational turns against the LLM port, running tool calls the
// model requests against the repository and verification ports. It implements
// ports.AgentRunner.
type Core struct {
	llm          ports.LLM
	convRepo     ports.ConversationRepo
	msgRepo      ports.MessageRepo
	companyRepo  ports.CompanyRepo
	contactRepo  ports.ContactRepo
	leadRepo     ports.LeadRepo
	sourcingRepo ports.SourcingRepo
	verifier     ports.CompanyDataProvider

	// mailer sends best-effort transactional email (lead confirmation, staff
	// notification, handoff notification). May be nil — email is then skipped.
	mailer ports.Mailer
	// activityLog appends audit rows for email/handoff side effects. May be nil.
	activityLog ports.ActivityLogRepo
	// staffNotify is the staff inbox address for internal notifications. Empty
	// disables staff email.
	staffNotify string
	// defaultLang is the locale used when the contact's language is unknown.
	defaultLang string
}

// Notifications bundles the optional email side-effect dependencies so they can
// be wired without widening the long-standing positional constructor. All fields
// are optional: a nil Mailer or empty StaffNotify disables the relevant email.
type Notifications struct {
	Mailer      ports.Mailer
	ActivityLog ports.ActivityLogRepo
	StaffNotify string
	DefaultLang string
}

// New constructs the agent core with the LLM port and the repository/service
// ports it needs to execute tools and persist conversation state. Email/handoff
// notification side effects are disabled (nil mailer); use NewWithNotifications
// to enable them.
func New(
	llm ports.LLM,
	convRepo ports.ConversationRepo,
	msgRepo ports.MessageRepo,
	companyRepo ports.CompanyRepo,
	contactRepo ports.ContactRepo,
	leadRepo ports.LeadRepo,
	sourcingRepo ports.SourcingRepo,
	verifier ports.CompanyDataProvider,
) *Core {
	return NewWithNotifications(llm, convRepo, msgRepo, companyRepo, contactRepo,
		leadRepo, sourcingRepo, verifier, Notifications{})
}

// NewWithNotifications is New plus the transactional-email/handoff side effects.
// The defaultLang falls back to RO when empty.
func NewWithNotifications(
	llm ports.LLM,
	convRepo ports.ConversationRepo,
	msgRepo ports.MessageRepo,
	companyRepo ports.CompanyRepo,
	contactRepo ports.ContactRepo,
	leadRepo ports.LeadRepo,
	sourcingRepo ports.SourcingRepo,
	verifier ports.CompanyDataProvider,
	n Notifications,
) *Core {
	lang := n.DefaultLang
	if lang == "" {
		lang = domain.LocaleRO
	}
	return &Core{
		llm:          llm,
		convRepo:     convRepo,
		msgRepo:      msgRepo,
		companyRepo:  companyRepo,
		contactRepo:  contactRepo,
		leadRepo:     leadRepo,
		sourcingRepo: sourcingRepo,
		verifier:     verifier,
		mailer:       n.Mailer,
		activityLog:  n.ActivityLog,
		staffNotify:  n.StaffNotify,
		defaultLang:  lang,
	}
}

// RunTurn processes one user message: it persists the message, advances the
// state machine, calls the LLM, executes any requested tools, feeds results back
// until the model produces final text, and returns that text. Assistant and tool
// messages are persisted as they are produced.
func (c *Core) RunTurn(ctx context.Context, conversationID string, userMessage string) (string, error) {
	return c.runTurn(ctx, conversationID, userMessage, true)
}

// RunTurnPersisted is RunTurn for callers that have already persisted the inbound
// user message (e.g. the async worker, which records it atomically with the
// provider-message-id idempotency claim). It is otherwise identical and reuses
// the persisted message as part of the rebuilt history.
func (c *Core) RunTurnPersisted(ctx context.Context, conversationID string, userMessage string) (string, error) {
	return c.runTurn(ctx, conversationID, userMessage, false)
}

func (c *Core) runTurn(ctx context.Context, conversationID string, userMessage string, appendUser bool) (string, error) {
	conv, err := c.convRepo.GetByID(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("load conversation: %w", err)
	}

	// Muted-bot guard (FR-6.1/6.3): once a conversation has been handed off to a
	// human (bot_active=false), the agent stays silent. Short-circuit BEFORE any
	// LLM call so a muted conversation never incurs model spend or auto-replies.
	if !conv.BotActive {
		// Persist the inbound user message (if the caller did not) so the human
		// sees it in the transcript, but do not run the model or reply.
		if appendUser && userMessage != "" {
			_ = c.msgRepo.Append(ctx, &domain.Message{
				ConversationID: conversationID,
				Role:           "user",
				Content:        userMessage,
			})
		}
		return "", nil
	}

	history, err := c.buildHistory(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("build history: %w", err)
	}

	// Persist the user message unless the caller already did (worker path).
	if appendUser {
		if err := c.msgRepo.Append(ctx, &domain.Message{
			ConversationID: conversationID,
			Role:           "user",
			Content:        userMessage,
		}); err != nil {
			return "", err
		}
	}

	// Advance state on first user message.
	if conv.State == domain.StateGreeting {
		_ = c.convRepo.UpdateState(ctx, conversationID, domain.StateQualifying)
	}

	// Seed the running transcript with prior history. When this turn's user
	// message was persisted by the caller (worker path), buildHistory already
	// includes it; otherwise (sync web path) append it now so the model sees it.
	messages := history
	if appendUser {
		messages = append(messages, ports.LLMMessage{Role: "user", Text: userMessage})
	}

	for round := 0; round < maxToolRounds; round++ {
		resp, err := c.llm.Complete(ctx, ports.LLMRequest{
			System:   systemPrompt,
			Messages: messages,
			Tools:    toolDefs(),
		})
		if err != nil {
			return "", fmt.Errorf("llm complete: %w", err)
		}

		// Persist the assistant turn (text and/or tool calls).
		if err := c.persistAssistant(ctx, conv.ID, resp); err != nil {
			return "", err
		}
		messages = append(messages, ports.LLMMessage{
			Role:      "assistant",
			Text:      resp.Text,
			ToolCalls: resp.ToolCalls,
		})

		if len(resp.ToolCalls) == 0 {
			return resp.Text, nil
		}

		// Execute each requested tool and feed the results back to the model.
		results := make([]ports.ToolResult, 0, len(resp.ToolCalls))
		for _, call := range resp.ToolCalls {
			result, execErr := c.executeTool(ctx, conv, call)
			if execErr != nil {
				result = map[string]any{"error": execErr.Error()}
			}

			// Persist the tool result message (keyed by tool name, as before).
			resultJSON, _ := json.Marshal(result)
			_ = c.msgRepo.Append(ctx, &domain.Message{
				ConversationID: conv.ID,
				Role:           "tool",
				Content:        string(resultJSON),
				ToolCallID:     call.Name,
			})

			results = append(results, ports.ToolResult{
				ID:      call.ID,
				Name:    call.Name,
				Content: result,
			})
		}

		messages = append(messages, ports.LLMMessage{
			Role:        "tool",
			ToolResults: results,
		})
	}

	return "", fmt.Errorf("agent: exceeded %d tool rounds without a final reply", maxToolRounds)
}

// persistAssistant stores the assistant turn. Tool calls are stored in our own
// neutral JSON (ports.ToolCall), never SDK types, so history can be rebuilt
// independent of the LLM provider.
func (c *Core) persistAssistant(ctx context.Context, conversationID string, resp *ports.LLMResponse) error {
	msg := &domain.Message{
		ConversationID: conversationID,
		Role:           "model",
		Content:        resp.Text,
	}
	if len(resp.ToolCalls) > 0 {
		raw, err := json.Marshal(resp.ToolCalls)
		if err != nil {
			return fmt.Errorf("marshal tool calls: %w", err)
		}
		msg.ToolCalls = raw
	}
	return c.msgRepo.Append(ctx, msg)
}

// buildHistory reconstructs the neutral transcript from persisted messages. It
// pairs each assistant tool-call message with the tool-result messages that
// follow it, so the LLM adapter receives a coherent call/result sequence.
func (c *Core) buildHistory(ctx context.Context, conversationID string) ([]ports.LLMMessage, error) {
	msgs, err := c.msgRepo.ListByConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	var history []ports.LLMMessage
	var pendingResults []ports.ToolResult

	flushResults := func() {
		if len(pendingResults) > 0 {
			history = append(history, ports.LLMMessage{Role: "tool", ToolResults: pendingResults})
			pendingResults = nil
		}
	}

	for _, m := range msgs {
		switch m.Role {
		case "user":
			flushResults()
			if m.Content == "" {
				continue
			}
			history = append(history, ports.LLMMessage{Role: "user", Text: m.Content})

		case "model":
			flushResults()
			am := ports.LLMMessage{Role: "assistant", Text: m.Content}
			if len(m.ToolCalls) > 0 {
				var calls []ports.ToolCall
				if json.Unmarshal(m.ToolCalls, &calls) == nil {
					am.ToolCalls = calls
				}
			}
			if am.Text == "" && len(am.ToolCalls) == 0 {
				continue
			}
			history = append(history, am)

		case "tool":
			var content map[string]any
			_ = json.Unmarshal([]byte(m.Content), &content)
			pendingResults = append(pendingResults, ports.ToolResult{
				ID:      m.ToolCallID,
				Name:    m.ToolCallID,
				Content: content,
			})
		}
	}
	flushResults()

	return history, nil
}
