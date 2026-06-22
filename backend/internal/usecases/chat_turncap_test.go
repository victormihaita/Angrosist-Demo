package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// countingMsgRepo is a MessageRepo fake whose user-message count is fixed, so the
// turn-cap branch is exercised without a database.
type countingMsgRepo struct{ userCount int }

func (r *countingMsgRepo) Append(ctx context.Context, msg *domain.Message) error { return nil }
func (r *countingMsgRepo) ListByConversation(ctx context.Context, conversationID string) ([]*domain.Message, error) {
	return nil, nil
}
func (r *countingMsgRepo) SeenProviderMsg(ctx context.Context, providerMsgID string) (bool, error) {
	return false, nil
}
func (r *countingMsgRepo) ClaimProviderMsg(ctx context.Context, conversationID, providerMsgID, content string) (bool, error) {
	return true, nil
}
func (r *countingMsgRepo) CountByConversationRole(ctx context.Context, conversationID, role string) (int, error) {
	if role != domain.MessageRoleUser {
		return 0, nil
	}
	return r.userCount, nil
}

// failIfCalledRunner fails the test if the agent (and therefore the LLM) is
// invoked — used to prove the cap short-circuits before any paid call.
type failIfCalledRunner struct{ t *testing.T }

func (r *failIfCalledRunner) RunTurn(ctx context.Context, convID, msg string) (string, error) {
	r.t.Fatal("agent RunTurn must NOT be called once the turn cap is reached")
	return "", nil
}

func TestChat_TurnCap_RejectsAtLimitWithoutLLM(t *testing.T) {
	conv := &chatConvRepo{}
	msgs := &countingMsgRepo{userCount: 40} // at the cap
	uc := NewChatUseCase(conv, &failIfCalledRunner{t: t}, passthroughLocker{}, nil).
		WithTurnCap(msgs, 40)

	_, err := uc.RunTurn(context.Background(), ChatRequest{ConversationID: "conv-1", Message: "more"})
	if !errors.Is(err, ErrTooManyTurns) {
		t.Fatalf("expected ErrTooManyTurns at the cap, got %v", err)
	}
}

func TestChat_TurnCap_AllowsUnderLimit(t *testing.T) {
	conv := &chatConvRepo{}
	msgs := &countingMsgRepo{userCount: 39} // under the cap
	runner := &chatRunner{}
	uc := NewChatUseCase(conv, runner, passthroughLocker{}, nil).WithTurnCap(msgs, 40)

	if _, err := uc.RunTurn(context.Background(), ChatRequest{ConversationID: "conv-1", Message: "more"}); err != nil {
		t.Fatalf("RunTurn under the cap should proceed: %v", err)
	}
	if !runner.called {
		t.Fatal("agent should have run under the cap")
	}
}

func TestChat_TurnCap_DisabledWhenZero(t *testing.T) {
	conv := &chatConvRepo{}
	msgs := &countingMsgRepo{userCount: 9999}
	runner := &chatRunner{}
	uc := NewChatUseCase(conv, runner, passthroughLocker{}, nil).WithTurnCap(msgs, 0) // 0 = unlimited

	if _, err := uc.RunTurn(context.Background(), ChatRequest{ConversationID: "conv-1", Message: "more"}); err != nil {
		t.Fatalf("disabled cap should never reject: %v", err)
	}
	if !runner.called {
		t.Fatal("agent should run when the cap is disabled")
	}
}

func TestChat_TurnCap_NewConversationNotCapped(t *testing.T) {
	conv := &chatConvRepo{}
	msgs := &countingMsgRepo{userCount: 9999} // would be over cap, but a new conv has no id
	runner := &chatRunner{}
	uc := NewChatUseCase(conv, runner, passthroughLocker{}, nil).WithTurnCap(msgs, 40)

	// No ConversationID => brand new conversation; the cap must not apply.
	if _, err := uc.RunTurn(context.Background(), ChatRequest{Message: "salut"}); err != nil {
		t.Fatalf("new conversation should never be capped: %v", err)
	}
	if conv.createCalled != 1 {
		t.Fatalf("expected a new conversation to be created, got %d", conv.createCalled)
	}
}

var _ ports.MessageRepo = (*countingMsgRepo)(nil)
