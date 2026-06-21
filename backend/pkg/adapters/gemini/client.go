package gemini

import (
	"context"
	"os"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// defaultModelName is the fallback used only when GEMINI_MODEL is unset.
// The model name is configuration, not a baked-in constant — set GEMINI_MODEL
// in the environment to change it without a code change (Hard Rule #1).
const defaultModelName = "gemini-2.5-flash"

// modelName resolves the Gemini model from the environment, falling back to a
// sane default for local/demo runs.
func modelName() string {
	if m := os.Getenv("GEMINI_MODEL"); m != "" {
		return m
	}
	return defaultModelName
}

var (
	clientOnce  sync.Once
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
	model := client.GenerativeModel(modelName())
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}
	model.Tools = agentTools
	return model
}
