package usecases

import (
	"context"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// chatConvRepo is a minimal ConversationRepo fake recording how a conversation
// was created (so the test can assert the vertical/intent passed through).
type chatConvRepo struct {
	created      *domain.Conversation
	createdVert  string
	createdInt   string
	createCalled int
}

func (r *chatConvRepo) Create(ctx context.Context, channel string) (*domain.Conversation, error) {
	return r.CreateWith(ctx, channel, "", "")
}
func (r *chatConvRepo) CreateWith(ctx context.Context, channel, vertical, intent string) (*domain.Conversation, error) {
	r.createCalled++
	r.createdVert, r.createdInt = vertical, intent
	r.created = &domain.Conversation{ID: "conv-1", Channel: channel, State: domain.StateGreeting,
		Vertical: vertical, Intent: intent, Extracted: map[string]any{}, BotActive: true}
	return r.created, nil
}
func (r *chatConvRepo) GetByID(ctx context.Context, id string) (*domain.Conversation, error) {
	if r.created != nil {
		return r.created, nil
	}
	return &domain.Conversation{ID: id, State: domain.StateQualifying, Extracted: map[string]any{}, BotActive: true}, nil
}
func (r *chatConvRepo) UpdateState(ctx context.Context, id string, s domain.ConversationState) error {
	return nil
}
func (r *chatConvRepo) UpdateExtracted(ctx context.Context, id string, e map[string]any) error {
	return nil
}
func (r *chatConvRepo) SetBotActive(ctx context.Context, id string, active bool) error { return nil }

// chatRunner is a no-op AgentRunner.
type chatRunner struct{ called bool }

func (r *chatRunner) RunTurn(ctx context.Context, convID, msg string) (string, error) {
	r.called = true
	return "ok", nil
}

// passthroughLocker runs the critical section inline.
type passthroughLocker struct{}

func (passthroughLocker) WithConversationLock(ctx context.Context, convID string, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestValidateVerticalIntent(t *testing.T) {
	cases := []struct {
		name             string
		vertical, intent string
		wantVert, wantIn string
		wantErr          bool
	}{
		{"omitted defaults to angrosist/buy", "", "", domain.VerticalAngrosist, domain.IntentBuy, false},
		{"pc sell ok", domain.VerticalPalletClearance, domain.IntentSell, domain.VerticalPalletClearance, domain.IntentSell, false},
		{"pc buy ok", domain.VerticalPalletClearance, domain.IntentBuy, domain.VerticalPalletClearance, domain.IntentBuy, false},
		{"invalid vertical rejected", "bogus", "buy", "", "", true},
		{"invalid intent rejected", "angrosist", "barter", "", "", true},
		{"skalyou not creatable from widget", "skalyou", "market_entry", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, in, err := ValidateVerticalIntent(tc.vertical, tc.intent)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got vertical=%q intent=%q", v, in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.wantVert || in != tc.wantIn {
				t.Fatalf("got (%q,%q), want (%q,%q)", v, in, tc.wantVert, tc.wantIn)
			}
		})
	}
}

func TestChat_NewConversationDefaultsAngrosistBuy(t *testing.T) {
	conv := &chatConvRepo{}
	uc := NewChatUseCase(conv, &chatRunner{}, passthroughLocker{}, nil)

	if _, err := uc.RunTurn(context.Background(), ChatRequest{Message: "Salut"}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if conv.createCalled != 1 {
		t.Fatalf("expected conversation created once, got %d", conv.createCalled)
	}
	if conv.createdVert != domain.VerticalAngrosist || conv.createdInt != domain.IntentBuy {
		t.Fatalf("expected angrosist/buy default, got %q/%q", conv.createdVert, conv.createdInt)
	}
}

func TestChat_NewConversationHonorsVerticalIntent(t *testing.T) {
	conv := &chatConvRepo{}
	uc := NewChatUseCase(conv, &chatRunner{}, passthroughLocker{}, nil)

	_, err := uc.RunTurn(context.Background(), ChatRequest{
		Message: "Vreau să vând", Vertical: domain.VerticalPalletClearance, Intent: domain.IntentSell,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if conv.createdVert != domain.VerticalPalletClearance || conv.createdInt != domain.IntentSell {
		t.Fatalf("expected palletclearance/sell, got %q/%q", conv.createdVert, conv.createdInt)
	}
}

func TestChat_InvalidVerticalRejectedBeforeCreate(t *testing.T) {
	conv := &chatConvRepo{}
	uc := NewChatUseCase(conv, &chatRunner{}, passthroughLocker{}, nil)

	_, err := uc.RunTurn(context.Background(), ChatRequest{Message: "x", Vertical: "bogus"})
	if err == nil {
		t.Fatalf("expected rejection for invalid vertical")
	}
	if conv.createCalled != 0 {
		t.Fatalf("conversation must not be created on invalid vertical, created=%d", conv.createCalled)
	}
}

// compile-time assertions that the fakes satisfy the ports they stand in for.
var (
	_ ports.ConversationRepo = (*chatConvRepo)(nil)
	_ ports.AgentRunner      = (*chatRunner)(nil)
	_ ports.Locker           = passthroughLocker{}
)
