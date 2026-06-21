package app

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"github.com/angrosist/demo/internal/agent"
	claudellm "github.com/angrosist/demo/internal/agent/llm/claude"
	geminillm "github.com/angrosist/demo/internal/agent/llm/gemini"
	pgadapter "github.com/angrosist/demo/internal/persistence/postgres"
	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/usecases"
	"github.com/angrosist/demo/internal/verification/anaf"
)

const DBTimeout = 5 * time.Second

type dbPinger struct {
	pool interface{ Ping(context.Context) error }
}

func (d *dbPinger) Ping(ctx context.Context) error { return d.pool.Ping(ctx) }

type Container struct {
	DB    interface{ Ping(context.Context) error }
	Chat  *usecases.ChatUseCase
	Leads *usecases.LeadUseCase
}

var (
	once      sync.Once
	container *Container
)

func Init() {
	once.Do(func() {
		godotenv.Load(findEnvFile())

		pool := pgadapter.GetPool()

		convRepo := pgadapter.NewConversationRepo()
		msgRepo := pgadapter.NewMessageRepo()
		companyRepo := pgadapter.NewCompanyRepo()
		contactRepo := pgadapter.NewContactRepo()
		leadRepo := pgadapter.NewLeadRepo()
		sourcingRepo := pgadapter.NewSourcingRepo()
		verifier := anaf.NewClient()

		llm := newLLM()
		runner := agent.New(
			llm,
			convRepo, msgRepo, companyRepo, contactRepo,
			leadRepo, sourcingRepo, verifier,
		)

		container = &Container{
			DB:    &dbPinger{pool: pool},
			Chat:  usecases.NewChatUseCase(convRepo, runner),
			Leads: usecases.NewLeadUseCase(leadRepo),
		}
	})
}

func GetContainer() *Container {
	Init()
	return container
}

// newLLM selects the LLM adapter based on the LLM_PROVIDER environment variable.
// Both adapters satisfy the same ports.LLM seam, so switching providers is a
// config change with zero impact on the agent core. Gemini is the default so the
// demo keeps working without an Anthropic key configured.
func newLLM() ports.LLM {
	switch os.Getenv("LLM_PROVIDER") {
	case "claude":
		return claudellm.New()
	default:
		return geminillm.New()
	}
}

func findEnvFile() string {
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	return "../.env"
}
