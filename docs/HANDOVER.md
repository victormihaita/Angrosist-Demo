# HANDOVER.md — operating & handover guide

> Everything an operator/developer needs to run, deploy, and maintain the Euro Intermed B2B platform. Pair with `README.md` (overview), `docs/BUILD_PLAN.md` (milestone status), `PROGRESS.md` (live status), and `docs/specs/*` (deep specs).

## 1. What's built (Phase 1)

| Milestone | Status | Summary |
|---|---|---|
| M1 Foundation | ✅ | `/cmd` + `/internal` hexagonal layout, full Phase-1 schema (26 migrations), local Docker stack, Terraform + CI/CD as code |
| M2 Agent core + widget | ✅ | Vendor-neutral `LLM` port (Gemini + Claude adapters), async runtime (Queue + worker + per-conversation lock + idempotency), SSE widget + typing |
| M3 Angrosist + dashboard | ✅ | Rich DemoANAF verification + CAEN→roles, auth/RBAC, full dashboard (pipeline/detail/offer/assign/directory/handoff/KPIs), email + handoff, file upload |
| M4 WhatsApp + PalletClearance + i18n | ✅ (buildable) | Vertical-aware agent, PalletClearance buyer/seller + mandatory seller photos, WhatsApp channel (built; live traffic gated on Meta), RO/EN UI |
| M5 GDPR + hardening | ◑ | Consent + cascade erasure + audit ✅, security audit + fixes ✅; **remaining:** full E2E suite, backup/restore drill (GCP) |

**Owner-gated / deferred:** live WhatsApp (Meta Business verification) · GCS FileStore adapter (at GCP provisioning) · WhatsApp 24h re-engagement templates + `wa.me` intent routing · automated retention sweep (Cloud Scheduler) · the M5 security residuals listed in `PROGRESS.md`.

## 2. Architecture (one screen)

- **Hexagonal.** `internal/domain` + `internal/agent` import zero infrastructure. Every external concern is a **port** with one adapter: `LLM` (Gemini/Claude), `CompanyDataProvider` (DemoANAF + raw-ANAF fallback), `Queue` (local/Cloud Tasks), `Locker` (Postgres advisory), `Broker` (in-process SSE; Redis later), `Channel`/`Replier` (web/WhatsApp), `Mailer` (log/SMTP), `FileStore` (localfs; GCS later), repositories, `ConsentRepo`/`ErasureRepo`.
- **Channel-agnostic agent.** The same flow engine + LLM port serves the web widget and WhatsApp; channel-specific code lives only in adapters. Adding a vertical = a `Flow` + prompt + sibling typed-request writer (no core change).
- **Async turns.** Channels ack fast → enqueue (`Queue`) → `cmd/worker` runs the turn under a per-conversation lock, idempotent by provider message id → reply delivered via the conversation's `Channel` (web→SSE, WhatsApp→Cloud API).
- **Three binaries, one module:** `cmd/server` (API + SSE + dashboard + webhooks), `cmd/worker` (Cloud Tasks push target), `cmd/migrate`.

Details: `docs/specs/ARCHITECTURE_DETAIL.md`.

## 3. Run locally (Docker — no GCP needed)

```bash
cp backend/.env.example backend/.env      # fill GEMINI_API_KEY (or set LLM_PROVIDER=claude + ANTHROPIC_API_KEY)
# set JWT_SECRET, ADMIN_EMAIL, ADMIN_PASSWORD in backend/.env (for the dashboard)
docker compose up --build
```
- Frontend → http://localhost:5173 · Backend health → http://localhost:8080/api/health
- The local Postgres URL is injected by compose (overrides `DATABASE_URL`); migrations run automatically before the API starts.
- Log in at `/login` with `ADMIN_EMAIL`/`ADMIN_PASSWORD` (bootstrapped on server start).
- Try a chat at `/chat` (Angrosist by default; `?vertical=palletclearance&intent=sell` for the seller flow). Set `ANAF_DEMO_MODE=true` to skip the live registry.

Without Docker: see `CONTRIBUTING.md` (go run ./cmd/migrate, ./cmd/server; npm run dev).

## 4. Environment variable inventory

Secrets live in a gitignored `.env` locally and in **Secret Manager** in prod — never in source. `*` = required for that feature.

**Backend — core**
| Var | Purpose |
|---|---|
| `DATABASE_URL` * | Postgres connection string |
| `PORT` | API server port (default 8080) |
| `JWT_SECRET` * | HS256 dashboard session signing (server refuses to issue tokens without it) |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | bootstrap an admin user on start (idempotent) |
| `CORS_ALLOWED_ORIGINS` | comma list or `*` (public widget); lock to dashboard origins in prod |

**LLM** — `LLM_PROVIDER` (gemini\|claude) · `GEMINI_API_KEY`*/`GEMINI_MODEL` · `ANTHROPIC_API_KEY`*/`CLAUDE_MODEL`/`CLAUDE_MAX_TOKENS`
**Verification** — `ANAF_PROVIDER` (demoanaf\|anaf) · `DEMOANAF_BASE_URL` · `ANAF_VAT_BASE_URL` (raw fallback) · `ANAF_DEMO_MODE`
**Async** — `QUEUE_PROVIDER` (local\|cloudtasks) · `WORKER_URL`* (cloudtasks) · `WORKER_AUTH_TOKEN` · `QUEUE_PUSH_TIMEOUT_SECONDS` · `WORKER_PORT`
**Email** — `MAIL_PROVIDER` (log\|smtp) · `SMTP_HOST/PORT/USER/PASSWORD`* (smtp) · `MAIL_FROM`* · `STAFF_NOTIFY_EMAIL` · `DEFAULT_LANG` (ro)
**Storage** — `FILESTORE_PROVIDER` (local\|gcs) · `FILESTORE_DIR` · `GCS_BUCKET` (gcs) · `MAX_UPLOAD_BYTES` · `MAX_PHOTOS_PER_CONVERSATION`
**GDPR** — `CONSENT_TEXT_VERSION`
**WhatsApp** (inert until set + Meta-verified) — `WHATSAPP_API_BASE` · `WHATSAPP_TOKEN`* · `WHATSAPP_PHONE_NUMBER_ID`* · `WHATSAPP_VERIFY_TOKEN`* · `WHATSAPP_APP_SECRET`*
**Frontend** — `VITE_API_URL` (backend base; baked at build) — never put secrets in `VITE_*`.
**Compose** — `POSTGRES_USER/PASSWORD/DB`.

## 5. Deploy to GCP (when ready)

Production is GCP, defined in Terraform. Full steps in `deploy/terraform/README.md`. Summary:
1. Create dev/staging/prod GCP projects (EU region) + billing; enable APIs.
2. Create the tfstate bucket; `terraform init/plan/apply` per env (Cloud SQL, Cloud Run api+worker, Cloud Tasks, GCS, Secret Manager, Artifact Registry, IAM).
3. Add secret values to Secret Manager (never in tfvars); configure Workload Identity Federation; set the GitHub Actions `vars`/`secrets` and `DEPLOY_ENABLED=true` to arm the CI deploy.
4. CI builds the image → Artifact Registry → Cloud Run; **migrations run as a deploy step**.
5. Provider switches for prod: `LLM_PROVIDER=claude`, `ANAF_PROVIDER=demoanaf`, `QUEUE_PROVIDER=cloudtasks`, `MAIL_PROVIDER=smtp`, `FILESTORE_PROVIDER=gcs` (build the GCS adapter — currently a deferred stub), `CORS_ALLOWED_ORIGINS` = real origins.

CI/CD: `.github/workflows/` (CI runs on push; GCP deploy jobs are dormant until `DEPLOY_ENABLED`).

## 6. Operational runbook

- **Migrations** — forward-only, additive (`backend/migrations/NNN_*.sql`); applied by `cmd/migrate` on deploy. Use `/new-migration`. Never reshape existing company/lead data.
- **Create/manage staff** — admins use `POST /api/users`; first admin via `ADMIN_*` bootstrap.
- **GDPR erasure** — admin: `POST /api/gdpr/erasure {contact_id|email}` → cascades the personal graph + deletes files, preserves public company data, redacts audit logs; returns a counts report.
- **Secrets rotation** — rotate in Secret Manager; redeploy. The owner's current dev keys (Gemini/Neon) move to client accounts at handover.
- **Backup/restore (do at provisioning)** — Cloud SQL automated backups + PITR + GCS object versioning are in Terraform; **run a restore drill** and document the result (M5 acceptance). Not exercisable locally.
- **Observability (M5/at provisioning)** — wire Cloud Logging + Monitoring + Sentry; logs are structured and PII-free.
- **WhatsApp go-live** — finish Meta Business verification, set `WHATSAPP_*`, point the Meta webhook at `/api/webhooks/whatsapp` with the verify token; signature is already enforced.

## 7. Security & GDPR posture

See `docs/specs/SECURITY.md` (threat model, RBAC matrix, audit catalog). Verified in the M5 audit: env-only secrets (no hardcoded), bcrypt + algorithm-pinned JWT, server-side RBAC on all dashboard/admin routes, constant-time WhatsApp signature verification, parameterized SQL, image-only sniffed uploads with traversal-safe keys, erasure preserves public data + redacts audit. **Open residuals (pre-production, tracked in `PROGRESS.md`):** conversation-ownership token on chat/SSE, rate-limit/max-turns on public chat, Cloud Tasks OIDC on `/worker/turn`, erasure blob-reconcile sweep.

## 8. Testing

- Backend: `cd backend && go test ./...` (unit + contract). Integration tests use a real Postgres and **skip without `DATABASE_URL`** — run them against a scratch cluster (the per-package tests cover the agent turn loop, verification, lead/typed-request creation, dashboard queries, offer/assign, handoff, photo gate, GDPR erasure, auth/RBAC, async lock/idempotency).
- Frontend: `cd frontend && npm run lint && npm run build && npm run build:widget`.
- **Remaining (M5):** an end-to-end suite (HTTP/Playwright) over the live Angrosist flow and a load test at target concurrency.

## 9. Claude tooling

Subagents, slash-commands, MCP (shadcn + Context7), and pre-commit hooks ship in `.claude/` — see root `CLAUDE.md` → "Claude tooling in this repo".
