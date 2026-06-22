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
	mockllm "github.com/angrosist/demo/internal/agent/llm/mock"
	"github.com/angrosist/demo/internal/api/authhttp"
	"github.com/angrosist/demo/internal/api/dashboardhttp"
	"github.com/angrosist/demo/internal/api/gdprhttp"
	"github.com/angrosist/demo/internal/api/ratelimit"
	"github.com/angrosist/demo/internal/api/uploadhttp"
	"github.com/angrosist/demo/internal/api/whatsapphttp"
	"github.com/angrosist/demo/internal/auth"
	"github.com/angrosist/demo/internal/broker"
	"github.com/angrosist/demo/internal/channels"
	"github.com/angrosist/demo/internal/channels/whatsapp"
	"github.com/angrosist/demo/internal/convtoken"
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
	// Broker is the real-time pub/sub seam for conversation events. The web reply
	// channel publishes to it; the SSE handler (cmd/server) subscribes to it.
	// In-process (single-instance) today; swap a Redis adapter for multi-instance
	// prod behind the same port.
	Broker ports.Broker

	// ConvToken issues/verifies stateless conversation-ownership tokens
	// (SECURITY.md §1.1). It is injected into the chat handler, the SSE stream
	// handler, and the public photo service so a guessed conversation_id cannot be
	// used to continue, read, or upload to someone else's conversation. The key
	// comes from CONVERSATION_TOKEN_SECRET, else is derived from JWT_SECRET.
	ConvToken *convtoken.Issuer

	// WhatsApp is the inbound WhatsApp webhook HTTP handler (GET verify + POST
	// signed). Its routes are PUBLIC (no bearer auth) but signature-verified; they
	// are registered unconditionally in cmd/server. The channel is inert until the
	// WHATSAPP_* env is set and the Meta number is verified.
	WhatsApp *whatsapphttp.Handler

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

	// GDPR is the admin-only data-subject-rights HTTP service (POST
	// /api/gdpr/erasure). Mounted behind the admin RBAC middleware in cmd/server.
	GDPR *gdprhttp.Service

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

	// RateLimiter is the per-client-IP token-bucket limiter applied (via the
	// ratelimit middleware) to the PUBLIC, expensive routes (POST /api/chat, POST
	// /api/conversations/{id}/photos). Configured by RATE_LIMIT_RPM/RATE_LIMIT_BURST;
	// a non-positive RPM yields a disabled (always-allow) limiter. Per-instance —
	// multi-instance prod relies on Cloudflare WAF or a Redis-backed limiter behind
	// the same middleware seam.
	RateLimiter *ratelimit.Limiter

	// WorkerAuthToken is the shared-secret bearer (WORKER_AUTH_TOKEN) the worker
	// push endpoint verifies and the Cloud Tasks adapter sends. Empty disables the
	// check (local/dev). cmd/worker reads it to build the authenticated handler.
	WorkerAuthToken string
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
		consentRepo := pgadapter.NewConsentRepo()
		erasureRepo := pgadapter.NewErasureRepo()
		verifier := newVerifier()

		// Auth: HS256 JWT issuer (fail fast if JWT_SECRET is unset) + user repo.
		tokens, err := auth.NewTokenIssuer(os.Getenv("JWT_SECRET"))
		if err != nil {
			log.Fatalf("container: %v", err)
		}
		authSvc := authhttp.NewService(userRepo, tokens)

		// Conversation-ownership token issuer (stateless, no DB). Key from
		// CONVERSATION_TOKEN_SECRET if set, else derived from JWT_SECRET with domain
		// separation — so no new required secret. Injected into the public chat/stream/
		// photo surfaces (SECURITY.md §1.1).
		convTokens := convtoken.New(os.Getenv("CONVERSATION_TOKEN_SECRET"), os.Getenv("JWT_SECRET"))

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
				Consent:      consentRepo,
			},
			agent.Notifications{
				Mailer:             mailer,
				ActivityLog:        activityRepo,
				StaffNotify:        os.Getenv("STAFF_NOTIFY_EMAIL"),
				DefaultLang:        os.Getenv("DEFAULT_LANG"),
				ConsentTextVersion: os.Getenv("CONSENT_TEXT_VERSION"),
			},
		)

		// Per-conversation lock: Postgres advisory lock in all real wirings (works
		// across worker instances). The in-memory locker exists for tests.
		locker := pgadapter.NewLocker()

		// Real-time event broker (single-instance in-process adapter). The web reply
		// channel publishes turn events; the SSE handler subscribes.
		eventBroker := broker.NewInProcess()

		// Channel-agnostic reply registry: resolve the per-conversation Replier by
		// channel. web -> SSE broker (behavior-preserving); whatsapp -> Cloud API
		// sender. Adding a channel = one more entry here (the swap recipe §5c).
		waSender := newWhatsAppSender()
		repliers := map[string]ports.Replier{
			channels.ChannelWeb: channels.NewWeb(eventBroker),
		}
		if waSender.Configured() {
			repliers[whatsapp.ChannelWhatsApp] = whatsapp.NewReplier(waSender, convRepo)
		}
		replierRegistry := channels.NewRegistry(repliers)

		// Async turn processor: lock + idempotency + agent turn. Shared by the
		// local queue handler and the cmd/worker HTTP endpoint.
		worker := usecases.NewTurnWorker(runner, locker, msgRepo, convRepo, replierRegistry)

		// Queue selection by QUEUE_PROVIDER. The local adapter binds the processor
		// as its in-process handler; Cloud Tasks pushes to the worker URL.
		q := newQueue(worker)

		// Inbound WhatsApp webhook (GET verify + POST signed). Public, signature-
		// verified; resolves/creates conversations by sender phone and enqueues a
		// turn. Inert (signature still enforced, sends are no-ops) until WHATSAPP_*
		// is configured and the Meta number is verified.
		whatsAppWebhook := whatsapphttp.NewHandler(
			whatsapphttp.Config{
				AppSecret:   os.Getenv("WHATSAPP_APP_SECRET"),
				VerifyToken: os.Getenv("WHATSAPP_VERIFY_TOKEN"),
			},
			convRepo,
			q,
		)

		leadsUC := usecases.NewLeadUseCase(leadRepo, userRepo, activityRepo)
		companiesUC := usecases.NewCompanyUseCase(companyRepo)

		// GDPR right-to-erasure: the use-case orchestrates the transactional DB
		// cascade (ErasureRepo) + best-effort blob deletion (FileStore) + the proof
		// audit row (ActivityLogRepo). companies/public data are preserved; the audit
		// trail is redacted, never deleted (SECURITY.md §7.4). FileStore is wired just
		// below; we build the service after it.

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
			convTokens,
			maxUploadBytes(), maxPhotosPerConversation(),
		)

		// GDPR erasure use-case + admin-only HTTP service (now that FileStore exists).
		erasureSvc := usecases.NewErasureService(erasureRepo, contactRepo, fileStore, activityRepo)
		gdprSvc := gdprhttp.NewService(erasureSvc)

		// Per-client-IP rate limiter for the public, expensive routes. Disabled when
		// RATE_LIMIT_RPM <= 0. A background goroutine reclaims idle buckets.
		rateLimiter := ratelimit.New(rateLimitRPM(), rateLimitBurst())
		rateLimiter.StartCleanup(0, make(chan struct{}))

		// Max-turns-per-conversation cost cap on the synchronous chat path. Enforced
		// before any paid LLM call; 0 (or unset) leaves it unlimited.
		chatUC := usecases.NewChatUseCase(convRepo, runner, locker, replierRegistry).
			WithTurnCap(msgRepo, maxTurnsPerConversation())

		container = &Container{
			DB:        &dbPinger{pool: pool},
			Chat:      chatUC,
			Leads:     leadsUC,
			Locker:    locker,
			Worker:    worker,
			Queue:     q,
			Broker:    eventBroker,
			ConvToken: convTokens,
			WhatsApp:  whatsAppWebhook,
			Auth:      authSvc,
			Dashboard: dashboardhttp.NewService(leadsUC, companiesUC),
			Upload:    uploadSvc,
			GDPR:      gdprSvc,
			Photos:    photoSvc,
			FileStore: fileStore,
			LocalFS:   localStore,

			RateLimiter:     rateLimiter,
			WorkerAuthToken: os.Getenv("WORKER_AUTH_TOKEN"),
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

// defaultRateLimitRPM / defaultMaxTurns are the safe baseline caps applied when
// the corresponding env vars are unset. They are generous enough not to disrupt a
// normal qualification conversation while still bounding abuse / LLM spend.
const (
	defaultRateLimitRPM = 60
	defaultMaxTurns     = 40
)

// rateLimitRPM reads RATE_LIMIT_RPM (requests/min/IP for the public expensive
// routes). Unset => defaultRateLimitRPM; 0 explicitly DISABLES the limiter; a
// negative/garbage value also disables it.
func rateLimitRPM() int {
	v := os.Getenv("RATE_LIMIT_RPM")
	if v == "" {
		return defaultRateLimitRPM
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultRateLimitRPM
	}
	return n
}

// rateLimitBurst reads RATE_LIMIT_BURST (the instantaneous burst capacity). A
// non-positive/unset value lets the limiter default it to the per-minute rate.
func rateLimitBurst() int {
	if v := os.Getenv("RATE_LIMIT_BURST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// maxTurnsPerConversation reads MAX_TURNS_PER_CONVERSATION (max USER turns before
// /api/chat refuses further input). Unset => defaultMaxTurns; 0 = unlimited.
func maxTurnsPerConversation() int {
	v := os.Getenv("MAX_TURNS_PER_CONVERSATION")
	if v == "" {
		return defaultMaxTurns
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultMaxTurns
	}
	return n
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

// newWhatsAppSender builds the WhatsApp Cloud API sender from the WHATSAPP_*
// environment (Secret Manager in prod). It is always constructed, but reports
// Configured()==false (and Send returns ErrNotConfigured) until WHATSAPP_TOKEN
// and WHATSAPP_PHONE_NUMBER_ID are set — so the channel is inert until the owner
// completes Meta Business verification and sets the secrets. Nothing is
// hardcoded; WHATSAPP_API_BASE defaults to the adapter's pinned Graph version.
func newWhatsAppSender() *whatsapp.Sender {
	return whatsapp.NewSender(whatsapp.SenderConfig{
		APIBase:       os.Getenv("WHATSAPP_API_BASE"),
		Token:         os.Getenv("WHATSAPP_TOKEN"),
		PhoneNumberID: os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
	})
}

// newLLM selects the LLM adapter based on the LLM_PROVIDER environment variable.
// All adapters satisfy the same ports.LLM seam, so switching providers is a config
// change with zero impact on the agent core. Gemini is the default so the demo
// keeps working without an Anthropic key configured.
//
//	gemini (default) — Google Gemini demo provider.
//	claude           — Anthropic Claude production provider.
//	mock             — TEST/LOCAL-only deterministic offline provider (internal/
//	                   agent/llm/mock). Never the default; selected explicitly so
//	                   the assembled stack can be exercised end-to-end without a
//	                   real model (see internal/e2e). Not for production.
func newLLM() ports.LLM {
	switch os.Getenv("LLM_PROVIDER") {
	case "claude":
		return claudellm.New()
	case "mock":
		return mockllm.New()
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
		log.Printf("bootstrap admin: upsert failed: %v", err)
		return
	}
	// Don't log the email (PII, SECURITY.md §8) — just that the bootstrap ran.
	log.Printf("bootstrap admin: ensured admin user from ADMIN_EMAIL")
}

func findEnvFile() string {
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	return "../.env"
}
