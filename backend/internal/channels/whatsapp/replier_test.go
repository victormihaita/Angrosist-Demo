package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

type fakeSender struct {
	to   string
	text string
	err  error
}

func (f *fakeSender) Send(_ context.Context, to, text string) error {
	f.to, f.text = to, text
	return f.err
}

type fakeResolver struct {
	phone string
	err   error
}

func (f *fakeResolver) ContactPhoneByConversation(context.Context, string) (string, error) {
	return f.phone, f.err
}

func TestReplier_Deliver_SendsToResolvedPhone(t *testing.T) {
	fs := &fakeSender{}
	r := newReplierWithSender(fs, &fakeResolver{phone: "40712345678"})

	conv := &domain.Conversation{ID: "conv-1", Channel: ChannelWhatsApp}
	err := r.Deliver(context.Background(), conv, ports.Event{Type: ports.EventMessage, Reply: "Multumesc!"})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if fs.to != "40712345678" {
		t.Errorf("sent to %q, want 40712345678", fs.to)
	}
	if fs.text != "Multumesc!" {
		t.Errorf("sent text %q, want Multumesc!", fs.text)
	}
}

func TestReplier_Deliver_EmptyReplyDropped(t *testing.T) {
	fs := &fakeSender{}
	r := newReplierWithSender(fs, &fakeResolver{phone: "4071"})
	if err := r.Deliver(context.Background(), &domain.Conversation{ID: "c"}, ports.Event{Reply: ""}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if fs.to != "" {
		t.Fatal("empty reply must not call Send")
	}
}

func TestReplier_Deliver_ResolverError(t *testing.T) {
	r := newReplierWithSender(&fakeSender{}, &fakeResolver{err: errors.New("no phone")})
	if err := r.Deliver(context.Background(), &domain.Conversation{ID: "c"}, ports.Event{Reply: "x"}); err == nil {
		t.Fatal("expected error when phone resolution fails")
	}
}

func TestReplier_TypingAndErrorAreNoops(t *testing.T) {
	fs := &fakeSender{}
	r := newReplierWithSender(fs, &fakeResolver{phone: "4071"})
	conv := &domain.Conversation{ID: "c", Channel: ChannelWhatsApp}
	if err := r.Typing(context.Background(), conv); err != nil {
		t.Fatalf("typing: %v", err)
	}
	if err := r.Error(context.Background(), conv, "agent error"); err != nil {
		t.Fatalf("error: %v", err)
	}
	if fs.to != "" {
		t.Fatal("typing/error must not send anything over WhatsApp")
	}
}
