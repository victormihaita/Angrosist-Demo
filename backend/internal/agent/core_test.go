package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// --- mock LLM ----------------------------------------------------------------

// scriptedLLM returns a queued response per Complete call, recording each
// request it received so tests can assert what the core sent.
type scriptedLLM struct {
	responses []*ports.LLMResponse
	calls     []ports.LLMRequest
	idx       int
}

func (l *scriptedLLM) Complete(ctx context.Context, req ports.LLMRequest) (*ports.LLMResponse, error) {
	l.calls = append(l.calls, req)
	if l.idx >= len(l.responses) {
		return nil, errors.New("scriptedLLM: no more responses")
	}
	resp := l.responses[l.idx]
	l.idx++
	return resp, nil
}

// --- mock repos --------------------------------------------------------------

type mockConvRepo struct {
	conv      *domain.Conversation
	states    []domain.ConversationState
	extracted []map[string]any
}

func (m *mockConvRepo) Create(ctx context.Context, channel string) (*domain.Conversation, error) {
	return m.conv, nil
}
func (m *mockConvRepo) GetByID(ctx context.Context, id string) (*domain.Conversation, error) {
	return m.conv, nil
}
func (m *mockConvRepo) UpdateState(ctx context.Context, id string, state domain.ConversationState) error {
	m.states = append(m.states, state)
	m.conv.State = state
	return nil
}
func (m *mockConvRepo) UpdateExtracted(ctx context.Context, id string, extracted map[string]any) error {
	m.extracted = append(m.extracted, extracted)
	m.conv.Extracted = extracted
	return nil
}

type mockMsgRepo struct {
	appended []*domain.Message
	history  []*domain.Message
}

func (m *mockMsgRepo) Append(ctx context.Context, msg *domain.Message) error {
	m.appended = append(m.appended, msg)
	return nil
}
func (m *mockMsgRepo) ListByConversation(ctx context.Context, conversationID string) ([]*domain.Message, error) {
	return m.history, nil
}
func (m *mockMsgRepo) SeenProviderMsg(ctx context.Context, providerMsgID string) (bool, error) {
	return false, nil
}
func (m *mockMsgRepo) ClaimProviderMsg(ctx context.Context, conversationID, providerMsgID, content string) (bool, error) {
	return true, nil
}

type mockCompanyRepo struct {
	byCUI    map[string]*domain.Company
	upserted []*domain.Company
}

func (m *mockCompanyRepo) GetByCUI(ctx context.Context, cui string) (*domain.Company, error) {
	if c, ok := m.byCUI[cui]; ok {
		return c, nil
	}
	return nil, errors.New("not found")
}
func (m *mockCompanyRepo) GetByRegNo(ctx context.Context, country, regNo string) (*domain.Company, error) {
	if c, ok := m.byCUI[regNo]; ok {
		return c, nil
	}
	return nil, errors.New("not found")
}
func (m *mockCompanyRepo) Upsert(ctx context.Context, company *domain.Company) error {
	m.upserted = append(m.upserted, company)
	if m.byCUI == nil {
		m.byCUI = map[string]*domain.Company{}
	}
	if company.ID == "" {
		company.ID = "company-1"
	}
	m.byCUI[company.CUI] = company
	return nil
}
func (m *mockCompanyRepo) ListPage(ctx context.Context, f domain.CompanyFilter) ([]*domain.CompanySummary, error) {
	return nil, nil
}
func (m *mockCompanyRepo) Detail(ctx context.Context, id string) (*domain.CompanyDetail, error) {
	return nil, nil
}

type mockContactRepo struct {
	created []*domain.Contact
	updated []*domain.Contact
}

func (m *mockContactRepo) Create(ctx context.Context, contact *domain.Contact) error {
	contact.ID = "contact-1"
	m.created = append(m.created, contact)
	return nil
}
func (m *mockContactRepo) Update(ctx context.Context, contact *domain.Contact) error {
	m.updated = append(m.updated, contact)
	return nil
}

type mockLeadRepo struct {
	created  []*domain.Lead
	existing *domain.Lead
}

func (m *mockLeadRepo) Create(ctx context.Context, lead *domain.Lead) error {
	lead.ID = "lead-1"
	m.created = append(m.created, lead)
	return nil
}
func (m *mockLeadRepo) GetByConversationID(ctx context.Context, convID string) (*domain.Lead, error) {
	if m.existing != nil {
		return m.existing, nil
	}
	return nil, errors.New("no lead")
}
func (m *mockLeadRepo) UpdateCompanyContact(ctx context.Context, leadID, companyID, contactID string) error {
	return nil
}
func (m *mockLeadRepo) List(ctx context.Context) ([]*domain.LeadSummary, error) { return nil, nil }
func (m *mockLeadRepo) GetByID(ctx context.Context, id string) (*domain.LeadDetail, error) {
	return nil, nil
}
func (m *mockLeadRepo) ListPage(ctx context.Context, f domain.LeadFilter) ([]*domain.LeadSummary, error) {
	return nil, nil
}
func (m *mockLeadRepo) Handoffs(ctx context.Context, f domain.LeadFilter) ([]*domain.HandoffItem, error) {
	return nil, nil
}
func (m *mockLeadRepo) UpdateOffer(ctx context.Context, leadID string, upd domain.OfferUpdate) (*domain.LeadSummary, error) {
	return nil, nil
}
func (m *mockLeadRepo) Assign(ctx context.Context, leadID string, userID *string) (*domain.LeadSummary, error) {
	return nil, nil
}
func (m *mockLeadRepo) StatusExists(ctx context.Context, status string) (bool, error) {
	return false, nil
}
func (m *mockLeadRepo) KPIs(ctx context.Context) (*domain.KPIs, error) { return nil, nil }

type mockSourcingRepo struct {
	created []*domain.SourcingRequest
	updated []*domain.SourcingRequest
}

func (m *mockSourcingRepo) Create(ctx context.Context, req *domain.SourcingRequest) error {
	m.created = append(m.created, req)
	return nil
}
func (m *mockSourcingRepo) UpdateByLeadID(ctx context.Context, req *domain.SourcingRequest) error {
	m.updated = append(m.updated, req)
	return nil
}

type mockVerifier struct {
	company *domain.Company
	err     error
}

func (m *mockVerifier) Verify(ctx context.Context, cui string) (*domain.Company, error) {
	return m.company, m.err
}

// --- helpers -----------------------------------------------------------------

func newTestCore(llm ports.LLM, conv *mockConvRepo, msg *mockMsgRepo, comp *mockCompanyRepo,
	contact *mockContactRepo, lead *mockLeadRepo, sourcing *mockSourcingRepo, verifier ports.CompanyDataProvider) *Core {
	return New(llm, conv, msg, comp, contact, lead, sourcing, verifier)
}

func countByRole(msgs []*domain.Message, role string) int {
	n := 0
	for _, m := range msgs {
		if m.Role == role {
			n++
		}
	}
	return n
}

// --- tests -------------------------------------------------------------------

func TestRunTurn_PlainText(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateGreeting}}
	msg := &mockMsgRepo{}
	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{Text: "Bună ziua! Cu ce vă pot ajuta?"},
	}}

	core := newTestCore(llm, conv, msg, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{})

	reply, err := core.RunTurn(context.Background(), "conv-1", "Salut")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if reply != "Bună ziua! Cu ce vă pot ajuta?" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	// Exactly one LLM call (no tools => no second round).
	if len(llm.calls) != 1 {
		t.Fatalf("expected 1 LLM call, got %d", len(llm.calls))
	}
	// System prompt + tools wired in.
	if llm.calls[0].System != systemPrompt {
		t.Fatalf("system prompt not passed to LLM")
	}
	if len(llm.calls[0].Tools) != 2 {
		t.Fatalf("expected 2 tool defs, got %d", len(llm.calls[0].Tools))
	}

	// Persisted: 1 user + 1 model message.
	if got := countByRole(msg.appended, "user"); got != 1 {
		t.Fatalf("expected 1 user message, got %d", got)
	}
	if got := countByRole(msg.appended, "model"); got != 1 {
		t.Fatalf("expected 1 model message, got %d", got)
	}

	// Greeting advances to qualifying on first user message.
	if conv.conv.State != domain.StateQualifying {
		t.Fatalf("expected state qualifying, got %s", conv.conv.State)
	}
}

func TestRunTurn_VerifyCompanyToolCall(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying}}
	msg := &mockMsgRepo{}
	comp := &mockCompanyRepo{}
	verifier := &mockVerifier{company: &domain.Company{
		CUI: "12345678", Name: "ACME SRL", Address: "Str. Test", County: "Cluj", IsActive: true,
	}}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		// First round: model requests verify_company.
		{ToolCalls: []ports.ToolCall{{ID: "verify_company", Name: "verify_company", Args: map[string]any{"cui": "12345678"}}}},
		// Second round: model produces final text after seeing the result.
		{Text: "Am verificat compania ACME SRL. Ce produse căutați?"},
	}}

	core := newTestCore(llm, conv, msg, comp, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, verifier)

	reply, err := core.RunTurn(context.Background(), "conv-1", "CUI-ul meu este 12345678")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if reply != "Am verificat compania ACME SRL. Ce produse căutați?" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	// Two LLM calls: initial + feed-result-back.
	if len(llm.calls) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(llm.calls))
	}

	// The second call must carry the tool result fed back.
	second := llm.calls[1]
	lastMsg := second.Messages[len(second.Messages)-1]
	if lastMsg.Role != "tool" || len(lastMsg.ToolResults) != 1 {
		t.Fatalf("expected last message to be a tool result, got %+v", lastMsg)
	}
	if found, _ := lastMsg.ToolResults[0].Content["found"].(bool); !found {
		t.Fatalf("expected tool result found=true, got %+v", lastMsg.ToolResults[0].Content)
	}

	// Verifier consulted; company persisted.
	if len(comp.upserted) != 1 {
		t.Fatalf("expected company upserted once, got %d", len(comp.upserted))
	}

	// State moved to verifying during the tool execution.
	sawVerifying := false
	for _, s := range conv.states {
		if s == domain.StateVerifying {
			sawVerifying = true
		}
	}
	if !sawVerifying {
		t.Fatalf("expected state to move to verifying, states=%v", conv.states)
	}

	// Persisted: user + model(tool-call) + tool result + model(final).
	if got := countByRole(msg.appended, "user"); got != 1 {
		t.Fatalf("expected 1 user message, got %d", got)
	}
	if got := countByRole(msg.appended, "tool"); got != 1 {
		t.Fatalf("expected 1 tool message, got %d", got)
	}
	if got := countByRole(msg.appended, "model"); got != 2 {
		t.Fatalf("expected 2 model messages, got %d", got)
	}

	// Extracted fields updated with verified company.
	if conv.conv.Extracted["company_name"] != "ACME SRL" {
		t.Fatalf("expected extracted company_name=ACME SRL, got %v", conv.conv.Extracted)
	}
}

func TestRunTurn_VerifyCompanyUnavailable(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying}}
	msg := &mockMsgRepo{}
	verifier := &mockVerifier{err: errors.New("ANAF service unavailable")}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{{ID: "verify_company", Name: "verify_company", Args: map[string]any{"cui": "999"}}}},
		{Text: "Verificăm manual. Ce produse căutați?"},
	}}

	core := newTestCore(llm, conv, msg, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, verifier)

	_, err := core.RunTurn(context.Background(), "conv-1", "CUI 999")
	if err != nil {
		t.Fatalf("RunTurn should not fail on unavailable verifier: %v", err)
	}

	// Tool result should signal unavailable=true, found=false (business outcome).
	result := llm.calls[1].Messages[len(llm.calls[1].Messages)-1].ToolResults[0].Content
	if found, _ := result["found"].(bool); found {
		t.Fatalf("expected found=false, got %+v", result)
	}
	if unavailable, _ := result["unavailable"].(bool); !unavailable {
		t.Fatalf("expected unavailable=true, got %+v", result)
	}
}

func TestBuildHistory_RebuildsToolCallsAndResults(t *testing.T) {
	msg := &mockMsgRepo{history: []*domain.Message{
		{Role: "user", Content: "Salut"},
		{Role: "model", Content: "", ToolCalls: []byte(`[{"ID":"verify_company","Name":"verify_company","Args":{"cui":"123"}}]`)},
		{Role: "tool", Content: `{"found":true,"active":true}`, ToolCallID: "verify_company"},
		{Role: "model", Content: "Compania este verificată."},
	}}
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying}}
	core := newTestCore(&scriptedLLM{}, conv, msg, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{})

	history, err := core.buildHistory(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("buildHistory: %v", err)
	}

	if len(history) != 4 {
		t.Fatalf("expected 4 history entries, got %d: %+v", len(history), history)
	}
	if history[0].Role != "user" || history[0].Text != "Salut" {
		t.Fatalf("entry 0 wrong: %+v", history[0])
	}
	if history[1].Role != "assistant" || len(history[1].ToolCalls) != 1 || history[1].ToolCalls[0].Name != "verify_company" {
		t.Fatalf("entry 1 wrong: %+v", history[1])
	}
	if history[2].Role != "tool" || len(history[2].ToolResults) != 1 || history[2].ToolResults[0].Name != "verify_company" {
		t.Fatalf("entry 2 wrong: %+v", history[2])
	}
	if history[3].Role != "assistant" || history[3].Text != "Compania este verificată." {
		t.Fatalf("entry 3 wrong: %+v", history[3])
	}
}
