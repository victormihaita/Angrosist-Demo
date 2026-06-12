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
	ConversationID string            `json:"conversation_id"`
	Reply          string            `json:"reply"`
	State          string            `json:"state"`
	Extracted      map[string]any    `json:"extracted"`
}

type ChatUseCase struct {
	convRepo ports.ConversationRepo
	runner   ports.AgentRunner
}

func NewChatUseCase(convRepo ports.ConversationRepo, runner ports.AgentRunner) *ChatUseCase {
	return &ChatUseCase{convRepo: convRepo, runner: runner}
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

	reply, err := uc.runner.RunTurn(ctx, convID, req.Message)
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

