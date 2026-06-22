package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// TestGetOrCreateByChannelPhone_CreatesThenReuses asserts the WhatsApp inbound
// resolution: the first inbound for a phone creates a contact + whatsapp
// conversation; a second inbound for the same phone reuses the open conversation;
// and the contact phone round-trips for outbound delivery.
func TestGetOrCreateByChannelPhone_CreatesThenReuses(t *testing.T) {
	requireDB(t)
	repo := NewConversationRepo()
	ctx := context.Background()

	// A unique phone per run so repeated test runs do not collide.
	var seq int64
	if err := GetPool().QueryRow(ctx, `SELECT (EXTRACT(EPOCH FROM clock_timestamp())*1000000)::bigint`).Scan(&seq); err != nil {
		t.Fatalf("seq: %v", err)
	}
	phone := "40700" + itoa(seq)

	first, err := repo.GetOrCreateByChannelPhone(ctx, "whatsapp", phone, domain.VerticalAngrosist, domain.IntentBuy)
	if err != nil {
		t.Fatalf("first get-or-create: %v", err)
	}
	if first.Channel != "whatsapp" {
		t.Fatalf("channel = %q, want whatsapp", first.Channel)
	}
	if first.Vertical != domain.VerticalAngrosist || first.Intent != domain.IntentBuy {
		t.Fatalf("vertical/intent = %s/%s, want angrosist/buy", first.Vertical, first.Intent)
	}

	second, err := repo.GetOrCreateByChannelPhone(ctx, "whatsapp", phone, domain.VerticalAngrosist, domain.IntentBuy)
	if err != nil {
		t.Fatalf("second get-or-create: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("same phone resolved to a different conversation: %s vs %s", first.ID, second.ID)
	}

	// The contact phone resolves for outbound delivery.
	got, err := repo.ContactPhoneByConversation(ctx, first.ID)
	if err != nil {
		t.Fatalf("contact phone: %v", err)
	}
	if got != phone {
		t.Fatalf("contact phone = %q, want %q", got, phone)
	}
}

// TestContactPhoneByConversation_NotFound asserts a conversation with no linked
// contact yields ErrNotFound.
func TestContactPhoneByConversation_NotFound(t *testing.T) {
	requireDB(t)
	repo := NewConversationRepo()
	convID := newConversation(t) // created without a contact link

	_, err := repo.ContactPhoneByConversation(context.Background(), convID)
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// itoa is a tiny base-10 int64 formatter (avoids importing strconv just for one
// call in tests).
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
