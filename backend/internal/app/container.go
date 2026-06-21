package app

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"github.com/angrosist/demo/internal/agent"
	claudellm "github.com/angrosist/demo/internal/agent/llm/claude"
	geminillm "github.com/angrosist/demo/internal/agent/llm/gemini"
	"github.com/angrosist/demo/internal/api/authhttp"
	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/broker"
	"github.com/angrosist/demo/internal/domain"
	pgadapter "github.com/angrosist/demo/internal/persistence/postgres"
	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/queue"
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

	// Locker serializes agent turns per conversation. The synchronous /api/chat
	// path runs its turn under this lock so concurrent same-conversation requests
	// serialize (M2 Epic 2.2), even though the transport stays synchronous for now.
	Locker ports.Locker
	// Worker is the async turn processor: lock + idempotency + agent turn. It is
	// shared by the local queue handler and the cmd/worker HTTP endpoint.
	Worker ports.TurnProcessor
	// Queue enqueues agent-turn jobs. Selected by QUEUE_PROVIDER (local | cloudtasks).
	Queue ports.Queue
	// Broker is the real-time pub/sub seam for conversation events. The chat
	// use-case and the worker publish to it; the SSE handler (cmd/server) subscribes
	// to it. In-process (single-instance) today; swap a Redis adapter for multi-
	// instance prod behind the same port.
	Broker ports.Broker

	// Auth is the staff/admin authentication + RBAC HTTP service: login handler,
	// admin user-management handlers, and the bearer-token / role middleware.
	Auth *authhttp.Service
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
		userRepo := pgadapter.NewUserRepo()
		verifier := newVerifier()

		// Auth: HS256 JWT issuer (fail fast if JWT_SECRET is unset) + user repo.
		tokens, err := auth.NewTokenIssuer(os.Getenv("JWT_SECRET"))
		if err != nil {
			log.Fatalf("container: %v", err)
		}
		authSvc := authhttp.NewService(userRepo, tokens)

		llm := newLLM()
		runner := agent.New(
			llm,
			convRepo, msgRepo, companyRepo, contactRepo,
			leadRepo, sourcingRepo, verifier,
		)

		// Per-conversation lock: Postgres advisory lock in all real wirings (works
		// across worker instances). The in-memory locker exists for tests.
		locker := pgadapter.NewLocker()

		// Real-time event broker (single-instance in-process adapter). The chat
		// use-case and the worker publish turn events; the SSE handler subscribes.
		eventBroker := broker.NewInProcess()

		// Async turn processor: lock + idempotency + agent turn. Shared by the
		// local queue handler and the cmd/worker HTTP endpoint.
		worker := usecases.NewTurnWorker(runner, locker, msgRepo, convRepo, eventBroker)

		// Queue selection by QUEUE_PROVIDER. The local adapter binds the processor
		// as its in-process handler; Cloud Tasks pushes to the worker URL.
		q := newQueue(worker)

		container = &Container{
			DB:     &dbPinger{pool: pool},
			Chat:   usecases.NewChatUseCase(convRepo, runner, locker, eventBroker),
			Leads:  usecases.NewLeadUseCase(leadRepo),
			Locker: locker,
			Worker: worker,
			Queue:  q,
			Broker: eventBroker,
			Auth:   authSvc,
		}

		// Idempotent admin bootstrap from ADMIN_EMAIL/ADMIN_PASSWORD (if both set).
		bootstrapAdmin(context.Background(), userRepo)
	})
}

// newQueue selects the Queue adapter from QUEUE_PROVIDER. "local" (default) runs
// the processor in-process so the demo works without external infra; "cloudtasks"
// pushes jobs to the worker URL (WORKER_URL) for durable async processing in prod.
func newQueue(processor ports.TurnProcessor) ports.Queue {
	switch os.Getenv("QUEUE_PROVIDER") {
	case "cloudtasks":
		ct, err := queue.NewCloudTasks(queue.CloudTasksConfig{
			WorkerURL: os.Getenv("WORKER_URL"),
			AuthToken: os.Getenv("WORKER_AUTH_TOKEN"),
			Timeout:   queueTimeout(),
		})
		if err != nil {
			log.Fatalf("container: cloudtasks queue: %v", err)
		}
		return ct
	default:
		return queue.NewLocal(processor)
	}
}

// queueTimeout reads QUEUE_PUSH_TIMEOUT_SECONDS (optional); 0 lets the adapter
// apply its default.
func queueTimeout() time.Duration {
	if v := os.Getenv("QUEUE_PUSH_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 0
}

func GetContainer() *Container {
	Init()
	return container
}

// newVerifier selects the company-verification adapter behind the
// ports.CompanyDataProvider seam, by ANAF_PROVIDER:
//
//	demoanaf (default) — the richer DemoANAF REST client (ONRC, administrators,
//	                     CAEN, derived roles); recommended for prod.
//	anaf               — the raw ANAF VAT-payer client, kept as a resilience
//	                     fallback behind the same port.
//
// ANAF_DEMO_MODE=true makes either client return deterministic demo data with no
// network call. Base URLs come from env (DEMOANAF_BASE_URL); nothing hardcoded.
func newVerifier() ports.CompanyDataProvider {
	switch os.Getenv("ANAF_PROVIDER") {
	case "anaf":
		return anaf.NewClient()
	default:
		return anaf.NewDemoANAFClient()
	}
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

// bootstrapAdmin creates (or updates) an admin user from ADMIN_EMAIL and
// ADMIN_PASSWORD when both are set. It is idempotent (UpsertByEmail) so repeated
// startups never duplicate the row. Secrets are never logged — only the fact that
// bootstrap ran and the (non-secret) email.
func bootstrapAdmin(ctx context.Context, users ports.UserRepo) {
	email := strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("bootstrap admin: hash password: %v", err)
		return
	}
	admin := &domain.User{
		Email:        email,
		Name:         "Administrator",
		Role:         domain.RoleAdmin,
		PasswordHash: hash,
	}
	if err := users.UpsertByEmail(ctx, admin); err != nil {
		log.Printf("bootstrap admin: upsert %s: %v", email, err)
		return
	}
	log.Printf("bootstrap admin: ensured admin user %s", email)
}

func findEnvFile() string {
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	return "../.env"
}
