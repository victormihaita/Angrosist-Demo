package usecases

import (
	"context"

	"github.com/angrosist/demo/internal/ports"
)

type ChatRequest struct {
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
}

type ChatResponse struct {
	ConversationID string         `json:"conversation_id"`
	Reply          string         `json:"reply"`
	State          string         `json:"state"`
	Extracted      map[string]any `json:"extracted"`
}

type ChatUseCase struct {
	convRepo ports.ConversationRepo
	runner   ports.AgentRunner
	locker   ports.Locker
}

// NewChatUseCase wires the synchronous chat path. The locker serializes turns
// per conversation: even though /api/chat stays synchronous for now (the async
// transport flip lands in Epic 2.3), concurrent requests for the same
// conversation run one at a time, preserving ordering and avoiding double-writes.
func NewChatUseCase(convRepo ports.ConversationRepo, runner ports.AgentRunner, locker ports.Locker) *ChatUseCase {
	return &ChatUseCase{convRepo: convRepo, runner: runner, locker: locker}
}

func (uc *ChatUseCase) RunTurn(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	convID := req.ConversationID
	if convID == "" {
		conv, err := uc.convRepo.Create(ctx, "web")
		if err != nil {
			return nil, err
		}
		convID = conv.ID
	}

	var reply string
	err := uc.locker.WithConversationLock(ctx, convID, func(ctx context.Context) error {
		r, err := uc.runner.RunTurn(ctx, convID, req.Message)
		if err != nil {
			return err
		}
		reply = r
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Reload conversation to get current state and extracted fields
	conv, err := uc.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}

	return &ChatResponse{
		ConversationID: convID,
		Reply:          reply,
		State:          string(conv.State),
		Extracted:      conv.Extracted,
	}, nil
}
