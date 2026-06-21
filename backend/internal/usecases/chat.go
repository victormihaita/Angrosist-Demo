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
	broker   ports.Broker
}

// NewChatUseCase wires the synchronous chat path. The locker serializes turns
// per conversation: even though /api/chat stays synchronous for now (the async
// transport flip lands in Epic 2.3), concurrent requests for the same
// conversation run one at a time, preserving ordering and avoiding double-writes.
//
// broker is the real-time pub/sub seam: the use-case publishes typing/message/
// error events so a subscribed SSE client receives the turn live. The HTTP
// response is unchanged — publishing is additive and best-effort. broker may be
// nil (publishing is then skipped) so tests can omit it.
func NewChatUseCase(convRepo ports.ConversationRepo, runner ports.AgentRunner, locker ports.Locker, broker ports.Broker) *ChatUseCase {
	return &ChatUseCase{convRepo: convRepo, runner: runner, locker: locker, broker: broker}
}

// publish is a nil-safe Broker.Publish so a missing broker is a no-op.
func (uc *ChatUseCase) publish(convID string, ev ports.Event) {
	if uc.broker != nil {
		uc.broker.Publish(convID, ev)
	}
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

	// Signal the typing indicator before the (potentially slow) turn work begins.
	uc.publish(convID, ports.Event{Type: ports.EventTyping})

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
		// Publish a safe error event (no internal detail / PII) for live clients,
		// then return the error to the HTTP caller unchanged.
		uc.publish(convID, ports.Event{Type: ports.EventError, Error: "agent error"})
		return nil, err
	}

	// Reload conversation to get current state and extracted fields
	conv, err := uc.convRepo.GetByID(ctx, convID)
	if err != nil {
		uc.publish(convID, ports.Event{Type: ports.EventError, Error: "agent error"})
		return nil, err
	}

	resp := &ChatResponse{
		ConversationID: convID,
		Reply:          reply,
		State:          string(conv.State),
		Extracted:      conv.Extracted,
	}

	// Publish the completed turn so a subscribed SSE client receives it live. The
	// HTTP response below is identical to before — this is purely additive.
	uc.publish(convID, ports.Event{
		Type:      ports.EventMessage,
		Reply:     resp.Reply,
		State:     resp.State,
		Extracted: resp.Extracted,
	})

	return resp, nil
}
