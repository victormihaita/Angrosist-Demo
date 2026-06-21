package email

import (
	"context"
	"net/smtp"
	"strings"
	"testing"

	"github.com/angrosist/demo/internal/domain"
)

func leadData() map[string]any {
	return map[string]any{
		"Product": "mere", "Quantity": "1000", "Unit": "kg", "Company": "ACME SRL",
		"ContactName": "Ion", "CUI": "12345678", "DeliveryLocation": "Cluj",
		"Phone": "0712", "Email": "buyer@example.com", "LeadID": "lead-1",
	}
}

func handoffData() map[string]any {
	return map[string]any{
		"Reason": "user_request", "Summary": "Cere un om.",
		"ConversationID": "conv-1", "LeadID": "lead-1",
	}
}

func TestRender_AllTemplatesBothLocales(t *testing.T) {
	cases := []struct {
		name string
		data map[string]any
		// substrings that must appear in the rendered body per locale.
		ro, en string
	}{
		{domain.EmailLeadConfirmation, leadData(), "Euro Intermed", "Euro Intermed"},
		{domain.EmailStaffLeadNotification, leadData(), "lead-1", "lead-1"},
		{domain.EmailHandoffNotification, handoffData(), "conv-1", "conv-1"},
	}
	for _, c := range cases {
		for _, loc := range []string{domain.LocaleRO, domain.LocaleEN} {
			r, err := render(c.name, loc, c.data)
			if err != nil {
				t.Fatalf("render %s/%s: %v", c.name, loc, err)
			}
			if strings.TrimSpace(r.Subject) == "" {
				t.Fatalf("%s/%s: empty subject", c.name, loc)
			}
			if strings.TrimSpace(r.HTML) == "" || strings.TrimSpace(r.Text) == "" {
				t.Fatalf("%s/%s: empty body", c.name, loc)
			}
			want := c.ro
			if loc == domain.LocaleEN {
				want = c.en
			}
			if !strings.Contains(r.Text, want) {
				t.Fatalf("%s/%s text missing %q: %q", c.name, loc, want, r.Text)
			}
		}
	}
}

func TestRender_KeyFieldsPresent(t *testing.T) {
	r, err := render(domain.EmailStaffLeadNotification, domain.LocaleEN, leadData())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"ACME SRL", "mere", "Cluj", "lead-1"} {
		if !strings.Contains(r.Text, want) {
			t.Fatalf("staff notification missing %q: %q", want, r.Text)
		}
	}
}

func TestRender_UnknownTemplate(t *testing.T) {
	if _, err := render("nope", domain.LocaleRO, nil); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestRender_UnknownLocaleFallsBackToRO(t *testing.T) {
	r, err := render(domain.EmailLeadConfirmation, "fr", leadData())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(r.Text, "Mulțumim") {
		t.Fatalf("expected RO fallback body, got %q", r.Text)
	}
}

func TestLogMailer_DoesNotError(t *testing.T) {
	m := NewLogMailer()
	err := m.Send(context.Background(), domain.EmailMessage{
		To:       []string{"buyer@example.com"},
		Template: domain.EmailLeadConfirmation,
		Locale:   domain.LocaleRO,
		Data:     leadData(),
	})
	if err != nil {
		t.Fatalf("log mailer send: %v", err)
	}
}

func TestLogMailer_UnknownTemplateErrors(t *testing.T) {
	m := NewLogMailer()
	if err := m.Send(context.Background(), domain.EmailMessage{
		To: []string{"x@y.z"}, Template: "nope", Locale: "ro",
	}); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestSMTPMailer_RequiresConfig(t *testing.T) {
	if _, err := NewSMTPMailer(SMTPConfig{}); err == nil {
		t.Fatal("expected error for missing host")
	}
	if _, err := NewSMTPMailer(SMTPConfig{Host: "h", Port: "587"}); err == nil {
		t.Fatal("expected error for missing from")
	}
}

func TestSMTPMailer_RendersAndDelivers(t *testing.T) {
	m, err := NewSMTPMailer(SMTPConfig{Host: "smtp.test", Port: "587", From: "no-reply@test"})
	if err != nil {
		t.Fatalf("new smtp mailer: %v", err)
	}

	var gotFrom string
	var gotTo []string
	var gotMsg []byte
	m.send = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotFrom, gotTo, gotMsg = from, to, msg
		return nil
	}

	err = m.Send(context.Background(), domain.EmailMessage{
		To:       []string{"buyer@example.com"},
		Template: domain.EmailLeadConfirmation,
		Locale:   domain.LocaleRO,
		Data:     leadData(),
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotFrom != "no-reply@test" || len(gotTo) != 1 || gotTo[0] != "buyer@example.com" {
		t.Fatalf("envelope wrong: from=%q to=%v", gotFrom, gotTo)
	}
	s := string(gotMsg)
	if !strings.Contains(s, "Subject:") || !strings.Contains(s, "text/html") || !strings.Contains(s, "text/plain") {
		t.Fatalf("MIME message malformed: %q", s)
	}
}

func TestSMTPMailer_SkipsEmptyRecipients(t *testing.T) {
	m, _ := NewSMTPMailer(SMTPConfig{Host: "h", Port: "587", From: "f@x"})
	called := false
	m.send = func(string, smtp.Auth, string, []string, []byte) error { called = true; return nil }
	if err := m.Send(context.Background(), domain.EmailMessage{Template: domain.EmailLeadConfirmation}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if called {
		t.Fatal("should not dial SMTP with no recipients")
	}
}
