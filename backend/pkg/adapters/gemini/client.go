package gemini

import (
	"context"
	"os"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const modelName = "gemini-2.5-flash"

var (
	clientOnce sync.Once
	genaiClient *genai.Client
)

func getClient(ctx context.Context) *genai.Client {
	clientOnce.Do(func() {
		var err error
		genaiClient, err = genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
		if err != nil {
			panic("gemini: failed to create client: " + err.Error())
		}
	})
	return genaiClient
}

func newModel(ctx context.Context) *genai.GenerativeModel {
	client := getClient(ctx)
	model := client.GenerativeModel(modelName)
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}
	model.Tools = agentTools
	return model
}
