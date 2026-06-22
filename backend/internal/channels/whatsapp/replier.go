package whatsapp

import (
	"context"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ChannelWhatsApp is the channel name stored on WhatsApp conversations and used
// to resolve this Replier from the registry.
const ChannelWhatsApp = "whatsapp"

// PhoneResolver returns the destination phone number for a WhatsApp conversation
// (the contact that opened it). It is a tiny port so the Replier does not depend
// on a concrete repository; the Postgres ConversationRepo satisfies it.
type PhoneResolver interface {
	// ContactPhoneByConversation returns the phone of the conversation's contact,
	// or ports.ErrNotFound when the conversation/contact/phone is missing.
	ContactPhoneByConversation(ctx context.Context, conversationID string) (string, error)
}

// textSender is the outbound seam the Replier needs (satisfied by *Sender). It is
// declared here so the Replier can be tested with a fake sender.
type textSender interface {
	Send(ctx context.Context, toPhone, text string) error
}

// Replier is the ports.Replier for the WhatsApp channel. It delivers a turn's
// reply text back to the conversation's contact phone via the Cloud API sender.
// Typing and Error are no-ops: WhatsApp has no live typing surface, and internal
// errors must never be sent to the end user.
type Replier struct {
	sender   textSender
	resolver PhoneResolver
}

var _ ports.Replier = (*Replier)(nil)

// NewReplier wires the WhatsApp Replier from a sender and a phone resolver.
func NewReplier(sender *Sender, resolver PhoneResolver) *Replier {
	return &Replier{sender: sender, resolver: resolver}
}

// newReplierWithSender is the test seam: it accepts the textSender interface so a
// fake sender can be injected.
func newReplierWithSender(sender textSender, resolver PhoneResolver) *Replier {
	return &Replier{sender: sender, resolver: resolver}
}

// Typing is a no-op on WhatsApp.
func (r *Replier) Typing(context.Context, *domain.Conversation) error { return nil }

// Deliver resolves the conversation's contact phone and sends ev.Reply through
// the Cloud API. An empty reply is dropped (nothing to send). It logs no PII.
func (r *Replier) Deliver(ctx context.Context, conv *domain.Conversation, ev ports.Event) error {
	if ev.Reply == "" {
		return nil
	}
	phone, err := r.resolver.ContactPhoneByConversation(ctx, conv.ID)
	if err != nil {
		return fmt.Errorf("whatsapp: resolve phone for conversation %s: %w", conv.ID, err)
	}
	if err := r.sender.Send(ctx, phone, ev.Reply); err != nil {
		return fmt.Errorf("whatsapp: deliver reply for conversation %s: %w", conv.ID, err)
	}
	return nil
}

// Error is a no-op on WhatsApp: internal errors are not surfaced to the end user.
func (r *Replier) Error(context.Context, *domain.Conversation, string) error { return nil }
