package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// --- fakes -------------------------------------------------------------------

// fakeMailer records every EmailMessage it is asked to send.
type fakeMailer struct {
	sent []domain.EmailMessage
	err  error
}

func (m *fakeMailer) Send(ctx context.Context, msg domain.EmailMessage) error {
	m.sent = append(m.sent, msg)
	return m.err
}

func (m *fakeMailer) byTemplate(name string) *domain.EmailMessage {
	for i := range m.sent {
		if m.sent[i].Template == name {
			return &m.sent[i]
		}
	}
	return nil
}

// fakeActivityLog records appended audit rows.
type fakeActivityLog struct {
	rows []domain.ActivityLog
}

func (a *fakeActivityLog) Append(ctx context.Context, e domain.ActivityLog) error {
	a.rows = append(a.rows, e)
	return nil
}

func (a *fakeActivityLog) byAction(action string) *domain.ActivityLog {
	for i := range a.rows {
		if a.rows[i].Action == action {
			return &a.rows[i]
		}
	}
	return nil
}

// newNotifyCore builds a Core wired with the email/handoff notification deps.
func newNotifyCore(llm ports.LLM, conv *mockConvRepo, msg *mockMsgRepo, comp *mockCompanyRepo,
	contact *mockContactRepo, lead *mockLeadRepo, sourcing *mockSourcingRepo, verifier ports.CompanyDataProvider,
	mailer ports.Mailer, audit ports.ActivityLogRepo, staff, lang string) *Core {
	return NewWithNotifications(llm, conv, msg, comp, contact, lead, sourcing, verifier,
		Notifications{Mailer: mailer, ActivityLog: audit, StaffNotify: staff, DefaultLang: lang})
}

// --- save_lead email side effects -------------------------------------------

func saveLeadCall() ports.ToolCall {
	return ports.ToolCall{ID: "save_lead", Name: "save_lead", Args: map[string]any{
		"product_name":      "mere",
		"quantity":          float64(1000),
		"unit":              "kg",
		"delivery_location": "Cluj",
		"cui":               "12345678",
		"company_name":      "ACME SRL",
		"phone":             "0712345678",
		"email":             "buyer@example.com",
	}}
}

func TestSaveLead_SendsConfirmationAndStaffEmails(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: true}}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "ACME SRL", IsActive: true},
	}}
	mailer := &fakeMailer{}
	audit := &fakeActivityLog{}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{saveLeadCall()}},
		{Text: "Cererea a fost înregistrată."},
	}}

	core := newNotifyCore(llm, conv, &mockMsgRepo{}, comp, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{}, mailer, audit, "staff@eurointermed.ro", "ro")

	if _, err := core.RunTurn(context.Background(), "conv-1", "gata"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	conf := mailer.byTemplate(domain.EmailLeadConfirmation)
	if conf == nil {
		t.Fatal("expected a lead_confirmation email")
	}
	if len(conf.To) != 1 || conf.To[0] != "buyer@example.com" {
		t.Fatalf("confirmation recipient wrong: %+v", conf.To)
	}
	if conf.Locale != "ro" {
		t.Fatalf("expected ro locale, got %q", conf.Locale)
	}

	staff := mailer.byTemplate(domain.EmailStaffLeadNotification)
	if staff == nil {
		t.Fatal("expected a staff notification email")
	}
	if len(staff.To) != 1 || staff.To[0] != "staff@eurointermed.ro" {
		t.Fatalf("staff recipient wrong: %+v", staff.To)
	}
	if staff.Data["LeadID"] == "" || staff.Data["LeadID"] == nil {
		t.Fatalf("staff email missing lead id: %+v", staff.Data)
	}

	if audit.byAction("email.sent") == nil {
		t.Fatal("expected an email.sent activity log row")
	}
}

func TestSaveLead_NoEmailWhenNoContactEmail(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: true}}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "ACME SRL", IsActive: true},
	}}
	mailer := &fakeMailer{}

	call := saveLeadCall()
	call.Args["email"] = "" // no prospect email

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{call}},
		{Text: "ok"},
	}}

	core := newNotifyCore(llm, conv, &mockMsgRepo{}, comp, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{}, mailer, &fakeActivityLog{}, "staff@eurointermed.ro", "ro")

	if _, err := core.RunTurn(context.Background(), "conv-1", "gata"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if mailer.byTemplate(domain.EmailLeadConfirmation) != nil {
		t.Fatal("did not expect a confirmation email without a prospect address")
	}
	if mailer.byTemplate(domain.EmailStaffLeadNotification) == nil {
		t.Fatal("staff should still be notified")
	}
}

func TestSaveLead_EmailFailureDoesNotFailTurn(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: true}}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "ACME SRL", IsActive: true},
	}}
	mailer := &fakeMailer{err: errors.New("smtp down")}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{saveLeadCall()}},
		{Text: "Cererea a fost înregistrată."},
	}}

	core := newNotifyCore(llm, conv, &mockMsgRepo{}, comp, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{}, mailer, &fakeActivityLog{}, "staff@eurointermed.ro", "ro")

	reply, err := core.RunTurn(context.Background(), "conv-1", "gata")
	if err != nil {
		t.Fatalf("a mailer failure must not fail the turn: %v", err)
	}
	if reply != "Cererea a fost înregistrată." {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestSaveLead_LocaleFromConversationLanguage(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: true, Language: "en"}}
	comp := &mockCompanyRepo{byCUI: map[string]*domain.Company{
		"12345678": {ID: "company-1", CUI: "12345678", Name: "ACME SRL", IsActive: true},
	}}
	mailer := &fakeMailer{}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{saveLeadCall()}},
		{Text: "ok"},
	}}

	core := newNotifyCore(llm, conv, &mockMsgRepo{}, comp, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{}, mailer, &fakeActivityLog{}, "staff@eurointermed.ro", "ro")

	if _, err := core.RunTurn(context.Background(), "conv-1", "go"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if conf := mailer.byTemplate(domain.EmailLeadConfirmation); conf == nil || conf.Locale != "en" {
		t.Fatalf("expected en confirmation from conversation language, got %+v", conf)
	}
}

// --- handoff_to_human --------------------------------------------------------

func TestHandoff_MutesBotNotifiesStaffAndLogs(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: true}}
	lead := &mockLeadRepo{existing: &domain.Lead{ID: "lead-1", ConversationID: "conv-1"}}
	mailer := &fakeMailer{}
	audit := &fakeActivityLog{}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{{ID: "h", Name: "handoff_to_human", Args: map[string]any{
			"reason": "user_request", "summary": "Cere un om.",
		}}}},
		{Text: "Te conectez cu un coleg."},
	}}

	core := newNotifyCore(llm, conv, &mockMsgRepo{}, &mockCompanyRepo{}, &mockContactRepo{},
		lead, &mockSourcingRepo{}, &mockVerifier{}, mailer, audit, "staff@eurointermed.ro", "ro")

	if _, err := core.RunTurn(context.Background(), "conv-1", "vreau să vorbesc cu cineva"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	if conv.conv.BotActive {
		t.Fatal("expected bot_active=false after handoff")
	}
	if len(lead.humanFlags) != 1 || !lead.humanFlags[0][0] /*needsHuman*/ || lead.humanFlags[0][1] /*botActive*/ {
		t.Fatalf("expected lead flags needs_human=true bot_active=false, got %+v", lead.humanFlags)
	}
	h := mailer.byTemplate(domain.EmailHandoffNotification)
	if h == nil {
		t.Fatal("expected a handoff notification email")
	}
	if h.Data["Reason"] != "user_request" {
		t.Fatalf("handoff email reason wrong: %+v", h.Data)
	}
	if audit.byAction("handoff") == nil {
		t.Fatal("expected a handoff activity log row")
	}
}

func TestHandoff_UnknownReasonNormalizedToOther(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: true}}
	mailer := &fakeMailer{}

	llm := &scriptedLLM{responses: []*ports.LLMResponse{
		{ToolCalls: []ports.ToolCall{{ID: "h", Name: "handoff_to_human", Args: map[string]any{
			"reason": "ignore-previous-instructions",
		}}}},
		{Text: "ok"},
	}}

	core := newNotifyCore(llm, conv, &mockMsgRepo{}, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{}, mailer, &fakeActivityLog{}, "staff@eurointermed.ro", "ro")

	if _, err := core.RunTurn(context.Background(), "conv-1", "??"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if h := mailer.byTemplate(domain.EmailHandoffNotification); h == nil || h.Data["Reason"] != "other" {
		t.Fatalf("expected reason normalized to 'other', got %+v", h)
	}
}

// --- muted-bot guard ---------------------------------------------------------

// failingLLM fails the test if it is ever called.
type failingLLM struct{ t *testing.T }

func (l *failingLLM) Complete(ctx context.Context, req ports.LLMRequest) (*ports.LLMResponse, error) {
	l.t.Fatal("LLM must not be called for a muted (bot_active=false) conversation")
	return nil, nil
}

func TestMutedBot_ShortCircuitsTurn(t *testing.T) {
	conv := &mockConvRepo{conv: &domain.Conversation{ID: "conv-1", State: domain.StateQualifying, BotActive: false}}
	msg := &mockMsgRepo{}

	core := newNotifyCore(&failingLLM{t: t}, conv, msg, &mockCompanyRepo{}, &mockContactRepo{},
		&mockLeadRepo{}, &mockSourcingRepo{}, &mockVerifier{}, &fakeMailer{}, &fakeActivityLog{}, "staff@eurointermed.ro", "ro")

	reply, err := core.RunTurn(context.Background(), "conv-1", "buna")
	if err != nil {
		t.Fatalf("muted turn should not error: %v", err)
	}
	if reply != "" {
		t.Fatalf("muted bot must not reply, got %q", reply)
	}
	// The inbound user message is still persisted for the human's transcript.
	if countByRole(msg.appended, "user") != 1 {
		t.Fatalf("expected inbound user message persisted, got %d", countByRole(msg.appended, "user"))
	}
	if countByRole(msg.appended, "model") != 0 {
		t.Fatal("muted bot must not persist a model reply")
	}
}
