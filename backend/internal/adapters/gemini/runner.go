package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

type Runner struct {
	convRepo    ports.ConversationRepo
	msgRepo     ports.MessageRepo
	companyRepo ports.CompanyRepo
	contactRepo ports.ContactRepo
	leadRepo    ports.LeadRepo
	sourcingRepo ports.SourcingRepo
	verifier    ports.CompanyVerifier
}

func NewRunner(
	convRepo ports.ConversationRepo,
	msgRepo ports.MessageRepo,
	companyRepo ports.CompanyRepo,
	contactRepo ports.ContactRepo,
	leadRepo ports.LeadRepo,
	sourcingRepo ports.SourcingRepo,
	verifier ports.CompanyVerifier,
) *Runner {
	return &Runner{
		convRepo:    convRepo,
		msgRepo:     msgRepo,
		companyRepo: companyRepo,
		contactRepo: contactRepo,
		leadRepo:    leadRepo,
		sourcingRepo: sourcingRepo,
		verifier:    verifier,
	}
}

func (r *Runner) RunTurn(ctx context.Context, conversationID string, userMessage string) (string, error) {
	conv, err := r.convRepo.GetByID(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("load conversation: %w", err)
	}

	history, err := r.buildHistory(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("build history: %w", err)
	}

	model := newModel(ctx)
	cs := model.StartChat()
	cs.History = history

	// Persist user message
	if err := r.msgRepo.Append(ctx, &domain.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        userMessage,
	}); err != nil {
		return "", err
	}

	// Advance state on first user message
	if conv.State == domain.StateGreeting {
		_ = r.convRepo.UpdateState(ctx, conversationID, domain.StateQualifying)
	}

	resp, err := cs.SendMessage(ctx, genai.Text(userMessage))
	if err != nil {
		return "", fmt.Errorf("gemini send: %w", err)
	}

	return r.processResponse(ctx, cs, conv, resp)
}

func (r *Runner) processResponse(
	ctx context.Context,
	cs *genai.ChatSession,
	conv *domain.Conversation,
	resp *genai.GenerateContentResponse,
) (string, error) {
	if len(resp.Candidates) == 0 {
		return "", errors.New("gemini: no candidates in response")
	}

	content := resp.Candidates[0].Content
	if content == nil {
		return "", errors.New("gemini: nil content")
	}

	// Persist model message — store text in Content so the transcript can display it.
	var textBuf strings.Builder
	for _, p := range content.Parts {
		if t, ok := p.(genai.Text); ok {
			textBuf.WriteString(string(t))
		}
	}
	modelMsg := &domain.Message{
		ConversationID: conv.ID,
		Role:           "model",
		Content:        textBuf.String(),
	}
	rawParts, _ := json.Marshal(content.Parts)
	modelMsg.ToolCalls = rawParts
	_ = r.msgRepo.Append(ctx, modelMsg)

	// Check for function calls
	for _, part := range content.Parts {
		fc, ok := part.(genai.FunctionCall)
		if !ok {
			continue
		}

		result, err := r.executeTool(ctx, conv, fc)
		if err != nil {
			result = map[string]any{"error": err.Error()}
		}

		// Persist tool result message
		resultJSON, _ := json.Marshal(result)
		_ = r.msgRepo.Append(ctx, &domain.Message{
			ConversationID: conv.ID,
			Role:           "tool",
			Content:        string(resultJSON),
			ToolCallID:     fc.Name,
		})

		// Send function response back to model
		nextResp, err := cs.SendMessage(ctx, genai.FunctionResponse{
			Name:     fc.Name,
			Response: result,
		})
		if err != nil {
			return "", fmt.Errorf("gemini tool response: %w", err)
		}

		return r.processResponse(ctx, cs, conv, nextResp)
	}

	// No more function calls — extract final text
	var sb strings.Builder
	for _, part := range content.Parts {
		if t, ok := part.(genai.Text); ok {
			sb.WriteString(string(t))
		}
	}
	return sb.String(), nil
}

func (r *Runner) executeTool(ctx context.Context, conv *domain.Conversation, fc genai.FunctionCall) (map[string]any, error) {
	switch fc.Name {
	case "verify_company":
		return r.toolVerifyCompany(ctx, conv, fc.Args)
	case "save_lead":
		return r.toolSaveLead(ctx, conv, fc.Args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", fc.Name)
	}
}

func (r *Runner) toolVerifyCompany(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	cui, _ := args["cui"].(string)
	cui = strings.TrimSpace(cui)

	_ = r.convRepo.UpdateState(ctx, conv.ID, domain.StateVerifying)

	company, err := r.verifier.Verify(ctx, cui)
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
			"found":    true,
			"active":   false,
			"name":     company.Name,
			"reason":   "Compania este inactivă în baza de date ANAF",
		}, nil
	}

	// Persist/update company record
	_ = r.companyRepo.Upsert(ctx, company)

	// Update extracted fields
	extracted := conv.Extracted
	if extracted == nil {
		extracted = make(map[string]any)
	}
	extracted["cui"] = cui
	extracted["company_name"] = company.Name
	_ = r.convRepo.UpdateExtracted(ctx, conv.ID, extracted)
	conv.Extracted = extracted

	return map[string]any{
		"found":    true,
		"active":   true,
		"name":     company.Name,
		"address":  company.Address,
		"county":   company.County,
	}, nil
}

func (r *Runner) toolSaveLead(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	cui, _ := args["cui"].(string)

	company, err := r.companyRepo.GetByCUI(ctx, cui)
	if err != nil {
		// ANAF was unavailable — upsert a minimal record from agent-collected args.
		company = &domain.Company{
			CUI:      cui,
			Name:     strArg(args["company_name"]),
			IsActive: true,
		}
		if upsertErr := r.companyRepo.Upsert(ctx, company); upsertErr != nil {
			return nil, fmt.Errorf("upsert company: %w", upsertErr)
		}
		// Reload to get the DB-assigned ID.
		company, err = r.companyRepo.GetByCUI(ctx, cui)
		if err != nil {
			return nil, fmt.Errorf("company not found after upsert: %w", err)
		}
	}

	contact := &domain.Contact{
		CompanyID: company.ID,
		Phone:     strArg(args["phone"]),
		Email:     strArg(args["email"]),
	}
	if err := r.contactRepo.Create(ctx, contact); err != nil {
		return nil, fmt.Errorf("create contact: %w", err)
	}

	lead := &domain.Lead{
		ConversationID: conv.ID,
		CompanyID:      company.ID,
		ContactID:      contact.ID,
		Status:         "new",
	}
	if err := r.leadRepo.Create(ctx, lead); err != nil {
		return nil, fmt.Errorf("create lead: %w", err)
	}

	qty := extractFloat(args["quantity"])
	req := &domain.SourcingRequest{
		LeadID:           lead.ID,
		ProductName:      strArg(args["product_name"]),
		Quantity:         qty,
		Unit:             strArg(args["unit"]),
		DeliveryLocation: strArg(args["delivery_location"]),
	}
	if err := r.sourcingRepo.Create(ctx, req); err != nil {
		return nil, fmt.Errorf("create sourcing request: %w", err)
	}

	// Update extracted and mark confirmed
	extracted := conv.Extracted
	if extracted == nil {
		extracted = make(map[string]any)
	}
	extracted["product_name"] = req.ProductName
	extracted["quantity"] = args["quantity"]
	extracted["unit"] = req.Unit
	extracted["delivery_location"] = req.DeliveryLocation
	extracted["phone"] = contact.Phone
	extracted["email"] = contact.Email
	_ = r.convRepo.UpdateExtracted(ctx, conv.ID, extracted)
	_ = r.convRepo.UpdateState(ctx, conv.ID, domain.StateConfirmed)

	return map[string]any{"saved": true, "lead_id": lead.ID}, nil
}

func (r *Runner) buildHistory(ctx context.Context, conversationID string) ([]*genai.Content, error) {
	msgs, err := r.msgRepo.ListByConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	var history []*genai.Content
	for _, m := range msgs {
		role := m.Role
		if role == "tool" {
			continue // tool results are embedded inline; handled by Gemini SDK
		}

		var parts []genai.Part

		if len(m.ToolCalls) > 0 && role == "model" {
			// Restore function call parts from persisted JSON
			var rawParts []json.RawMessage
			if json.Unmarshal(m.ToolCalls, &rawParts) == nil {
				for _, rp := range rawParts {
					var fc genai.FunctionCall
					if json.Unmarshal(rp, &fc) == nil && fc.Name != "" {
						parts = append(parts, fc)
						continue
					}
					var txt string
					if json.Unmarshal(rp, &txt) == nil {
						parts = append(parts, genai.Text(txt))
					}
				}
			}
		} else if m.Content != "" {
			parts = []genai.Part{genai.Text(m.Content)}
		}

		if len(parts) == 0 {
			continue
		}

		history = append(history, &genai.Content{Role: role, Parts: parts})
	}
	return history, nil
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
