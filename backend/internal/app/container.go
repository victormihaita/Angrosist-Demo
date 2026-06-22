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
	"github.com/angrosist/demo/internal/api/dashboardhttp"
	"github.com/angrosist/demo/internal/api/uploadhttp"
	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/broker"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/email"
	pgadapter "github.com/angrosist/demo/internal/persistence/postgres"
	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/queue"
	"github.com/angrosist/demo/internal/storage/gcs"
	"github.com/angrosist/demo/internal/storage/localfs"
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

	// Dashboard is the authenticated dashboard data HTTP service (leads pipeline,
	// lead detail, offer tracking, assignment, B2B directory, handoff queue, KPIs).
	// Its routes are mounted behind Auth.Auth.Require in cmd/server.
	Dashboard *dashboardhttp.Service

	// Upload is the validated multipart document-upload HTTP service (POST
	// /api/upload). Mounted behind the staff auth middleware in cmd/server.
	Upload *uploadhttp.Service

	// Photos is the PUBLIC, conversation-scoped seller-photo upload service
	// (POST /api/conversations/{id}/photos). Mounted WITHOUT auth (the widget is
	// public) but scoped to PalletClearance seller conversations. It backs the
	// seller-photo blocking gate enforced in the agent seller submit.
	Photos *uploadhttp.PhotoService

	// FileStore is the binary object-storage seam (localfs in dev/docker, GCS at
	// provisioning). Selected by FILESTORE_PROVIDER.
	FileStore ports.FileStore

	// LocalFS is the concrete local-filesystem store when FILESTORE_PROVIDER=local
	// (nil otherwise). cmd/server reads it to register the GET /uploads/{key}
	// file-serving route; prod serves objects via GCS signed URLs instead.
	LocalFS *localfs.Store
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
		listingRepo := pgadapter.NewListingRepo()
		buyerProfileRepo := pgadapter.NewBuyerProfileRepo()
		userRepo := pgadapter.NewUserRepo()
		activityRepo := pgadapter.NewActivityLogRepo()
		docRepo := pgadapter.NewDocumentRepo()
		verifier := newVerifier()

		// Auth: HS256 JWT issuer (fail fast if JWT_SECRET is unset) + user repo.
		tokens, err := auth.NewTokenIssuer(os.Getenv("JWT_SECRET"))
		if err != nil {
			log.Fatalf("container: %v", err)
		}
		authSvc := authhttp.NewService(userRepo, tokens)

		llm := newLLM()
		mailer := newMailer()
		// Vertical-aware agent: the FlowRegistry selects the per-(vertical,intent)
		// flow, and the per-vertical typed-request repos back the flows' submit paths.
		runner := agent.NewWithFlows(
			llm,
			convRepo, msgRepo, companyRepo, contactRepo,
			leadRepo, sourcingRepo, verifier,
			agent.NewFlowRegistry(),
			agent.Repos{
				Listing:      listingRepo,
				BuyerProfile: buyerProfileRepo,
				Document:     docRepo,
			},
			agent.Notifications{
				Mailer:      mailer,
				ActivityLog: activityRepo,
				StaffNotify: os.Getenv("STAFF_NOTIFY_EMAIL"),
				DefaultLang: os.Getenv("DEFAULT_LANG"),
			},
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

		leadsUC := usecases.NewLeadUseCase(leadRepo, userRepo, activityRepo)
		companiesUC := usecases.NewCompanyUseCase(companyRepo)

		// Document storage: FileStore behind the port (localfs in dev/docker, GCS
		// stub until provisioning) + the document index repo + the upload service.
		fileStore, localStore := newFileStore()
		uploadSvc := uploadhttp.NewService(fileStore, docRepo, maxUploadBytes())

		// Public, conversation-scoped seller-photo upload (PalletClearance). It reuses
		// the same FileStore + DocumentRepo behind the ports, plus a guard that limits
		// public writes to existing seller conversations. Backs the seller-photo gate.
		photoSvc := uploadhttp.NewPhotoService(
			fileStore, docRepo,
			sellerConversationGuard{conv: convRepo},
			maxUploadBytes(), maxPhotosPerConversation(),
		)

		container = &Container{
			DB:        &dbPinger{pool: pool},
			Chat:      usecases.NewChatUseCase(convRepo, runner, locker, eventBroker),
			Leads:     leadsUC,
			Locker:    locker,
			Worker:    worker,
			Queue:     q,
			Broker:    eventBroker,
			Auth:      authSvc,
			Dashboard: dashboardhttp.NewService(leadsUC, companiesUC),
			Upload:    uploadSvc,
			Photos:    photoSvc,
			FileStore: fileStore,
			LocalFS:   localStore,
		}

		// Idempotent admin bootstrap from ADMIN_EMAIL/ADMIN_PASSWORD (if both set).
		bootstrapAdmin(context.Background(), userRepo)
	})
}

// newFileStore selects the FileStore adapter behind the ports.FileStore seam by
// FILESTORE_PROVIDER:
//
//	local (default) — internal/storage/localfs; writes under FILESTORE_DIR and
//	                  serves bytes via the GET /uploads/{key} route. Runs in docker
//	                  compose / locally with no cloud credentials.
//	gcs             — internal/storage/gcs; DEFERRED to provisioning. The stub
//	                  fails loudly (ErrNotConfigured) rather than faking success;
//	                  the real adapter (signed URLs, private bucket) lands then.
//
// It returns the port plus the concrete *localfs.Store (nil for gcs) so cmd/server
// can register the dev file-serving route. Nothing is hardcoded — directory and
// bucket come from env.
func newFileStore() (ports.FileStore, *localfs.Store) {
	switch os.Getenv("FILESTORE_PROVIDER") {
	case "gcs":
		return gcs.New(gcs.Config{Bucket: os.Getenv("GCS_BUCKET")}), nil
	default:
		store := localfs.New(os.Getenv("FILESTORE_DIR"))
		return store, store
	}
}

// maxUploadBytes reads MAX_UPLOAD_BYTES (optional); a non-positive/unset value
// lets the upload service apply its default cap.
func maxUploadBytes() int64 {
	if v := os.Getenv("MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// maxPhotosPerConversation reads MAX_PHOTOS_PER_CONVERSATION (optional); a
// non-positive/unset value lets the photo service apply its default per-conversation
// cap. It limits abuse of the public seller-photo endpoint.
func maxPhotosPerConversation() int {
	if v := os.Getenv("MAX_PHOTOS_PER_CONVERSATION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
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

// newMailer selects the Mailer adapter behind the ports.Mailer seam by
// MAIL_PROVIDER:
//
//	log  (default) — renders and logs a non-PII summary instead of sending, so
//	                 docker compose / tests / the demo need no SMTP credentials.
//	smtp           — delivers over SMTP+STARTTLS using SMTP_HOST/PORT/USER/
//	                 PASSWORD + MAIL_FROM. Any EU provider that speaks SMTP
//	                 (Brevo/Mailgun/SendGrid) works; no vendor SDK is added.
//
// All values come from env (Secret Manager in prod); nothing is hardcoded.
func newMailer() ports.Mailer {
	switch os.Getenv("MAIL_PROVIDER") {
	case "smtp":
		m, err := email.NewSMTPMailer(email.SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     os.Getenv("SMTP_PORT"),
			User:     os.Getenv("SMTP_USER"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("MAIL_FROM"),
		})
		if err != nil {
			log.Fatalf("container: smtp mailer: %v", err)
		}
		return m
	default:
		return email.NewLogMailer()
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
