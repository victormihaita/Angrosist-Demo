package app

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"github.com/angrosist/demo/internal/adapters/anaf"
	"github.com/angrosist/demo/internal/adapters/gemini"
	pgadapter "github.com/angrosist/demo/internal/adapters/postgres"
	"github.com/angrosist/demo/internal/usecases"
)

const DBTimeout = 5 * time.Second

type dbPinger struct{ pool interface{ Ping(context.Context) error } }

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

		convRepo    := pgadapter.NewConversationRepo()
		msgRepo     := pgadapter.NewMessageRepo()
		companyRepo := pgadapter.NewCompanyRepo()
		contactRepo := pgadapter.NewContactRepo()
		leadRepo    := pgadapter.NewLeadRepo()
		sourcingRepo := pgadapter.NewSourcingRepo()
		verifier    := anaf.NewClient()

		runner := gemini.NewRunner(
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

func findEnvFile() string {
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	return "../.env"
}
