# ARCHITECTURE.md — Clean Architecture & Deployment

## 1. Principles

- **Hexagonal (ports & adapters).** The domain (entities + use-cases) depends on nothing external. Every external concern — HTTP, the LLM, WhatsApp, the web widget, DemoANAF, email, GCS, Postgres, the queue — is a **port** (interface) with one **adapter**.
- **Channel-agnostic agent core.** Channels are adapters that normalize inbound events into one `Conversation`/`Message` model and send outbound through a `Channel` port. Qualification logic is written once.
- **Tools, not direct access.** The LLM emits tool calls; our code executes them against ports. The LLM never holds a DB handle or an API key.
- **Async agent turns.** Webhooks/WS ack immediately and enqueue; a worker does the slow LLM/tool work. Nothing user-facing blocks on the LLM.
- **12-factor, stateless.** State lives in Cloud SQL / GCS (+ Redis for hot/ephemeral). Instances are disposable.

## 2. Logical components

```
            ┌─────────── Channels (adapters) ───────────┐
 WhatsApp ──┤ whatsapp adapter (webhook, signature,      │
            │                   24h window, templates)   │──┐
 Web widget ┤ webwidget adapter (WS/SSE, sessions)       │  │  normalize → Conversation/Message
            └────────────────────────────────────────────┘  │
                                                             ▼
                                   ┌──────────── Agent core ────────────┐
                                   │ flow engine (per-vertical fields)   │
                                   │ + LLM port (Claude)                 │
                                   │ + tools: verify_company,            │
                                   │   upload_media, submit_lead,        │
                                   │   handoff_to_human, [P2] classify   │
                                   └──────────────┬──────────────────────┘
                                                  ▼ (use-cases)
                                   ┌──────────── Domain ─────────────────┐
                                   │ companies, contacts, categories,    │
                                   │ leads (+ typed requests), listings, │
                                   │ consents, audit, [P2] matching,     │
                                   │ providers, offers, market-entry     │
                                   └───┬───────┬───────┬───────┬─────────┘
                                       ▼       ▼       ▼       ▼
                                  Postgres   GCS    DemoANAF  Email   (adapters via ports)
                                  (Cloud SQL)(files)(verify) (mailer)

 Dashboard / Provider portal / Client page  ──HTTP──►  Dashboard API  ──► Domain
 Cloud Tasks (queue)  ──► worker endpoint ──► Agent core
 Cloud Scheduler (cron) ──► job endpoints (timeouts, reminders, matching)
```

## 3. Ports (interfaces) to define

`Channel` (send), `LLM` (complete/stream + tool-calling), `CompanyDataProvider` (DemoANAF), `FileStore` (GCS), `Mailer` (email), `Queue` (Cloud Tasks enqueue), `Repositories` (per aggregate), `Clock`, `IDGen`. Each has exactly one production adapter and is mocked in tests.

## 4. Agent runtime (the critical path)

1. Channel adapter receives an inbound message; verifies authenticity (WhatsApp signature); **acks fast** (HTTP 200 for WhatsApp).
2. Dedupe by provider message ID (Redis/Postgres). Enqueue a job (Cloud Tasks) with the conversation + message.
3. Worker pulls the job, takes a **per-conversation lock**, loads conversation state.
4. Flow engine determines missing required fields for the vertical; calls the **LLM** with state + system prompt; the LLM extracts fields and/or returns a tool call.
5. Tool calls execute against ports (verify company, upload media to GCS, submit lead, handoff). Results feed back to the LLM if needed.
6. Update state; send the reply via the channel adapter. On completion, create the normalized lead and (deferred handoff) notify staff.

Workers are stateless; scale by adding instances. Cloud Tasks gives durability + retry; ordering per conversation is preserved by the per-conversation lock.

## 5. Tech stack

Backend **Go** (Docker image). **PostgreSQL** (Cloud SQL). **Redis** (Memorystore; optional early). Queue **Cloud Tasks**; cron **Cloud Scheduler**. **GCS** for files. **Secret Manager** for secrets. **Artifact Registry** for images. Frontend **React/TypeScript** on **Firebase Hosting** (static) or **Cloud Run** (SSR). **Claude** via API. **WhatsApp Cloud API**. **DemoANAF** for verification. External **email provider** (SendGrid/Mailgun/Brevo, EU region). **Terraform** for all infra. **Cloud Logging/Monitoring + Sentry**.

## 6. GCP deployment topology

Everything on GCP. Backend is a Docker image.

| Concern | GCP service |
|---|---|
| Backend container (API + agent + worker endpoints) | **Cloud Run** (Docker image; `min-instances ≥ 1` so it's warm; WebSocket supported) |
| Async queue | **Cloud Tasks** → push to the worker endpoint on Cloud Run |
| Cron (timeouts, reminders, [P2] matching) | **Cloud Scheduler** → job endpoints |
| Database | **Cloud SQL for PostgreSQL** (automated backups + PITR; HA optional) |
| Cache / locks / dedupe | **Memorystore for Redis** (optional until needed) |
| Object storage (photos, offers) | **Cloud Storage**, signed URLs |
| Secrets | **Secret Manager** |
| Image registry | **Artifact Registry** |
| Frontend (dashboard, widget, [P2] portal, client page) | **Firebase Hosting** (static) or **Cloud Run** (SSR) |
| CI/CD | **GitHub Actions** (or Cloud Build) → Artifact Registry → Cloud Run |
| Edge / DNS / WAF / CDN | **Cloudflare** in front (TLS, caching for the widget, protection) |
| Logs / metrics / errors | **Cloud Logging + Cloud Monitoring + Sentry** |

> Alternative for the backend: a single **Compute Engine** VM running the Docker image via docker-compose (simplest "one instance" mental model). Cloud Run is the default because it's managed, autoscaling, and keeps everything serverless. Pick one and keep it consistent.

**Non-GCP runtime dependencies (unavoidable):** the LLM API (Claude), WhatsApp Cloud API, DemoANAF, and the transactional email provider.

## 7. Environments, config, secrets

- `development`, `staging`, `production` — separate GCP projects (or strictly separated resources). Never deploy from a laptop; deploy from CI.
- Config via env vars; secrets via Secret Manager (DB creds, LLM key, WhatsApp token/app-secret/verify-token, email key). Nothing secret in git.

## 8. Data flow & resilience

- All durable state in Cloud SQL + GCS. Redis holds only cache/locks/dedupe (rebuildable). A Cloud Run revision dying loses nothing.
- Backups: Cloud SQL automated + PITR; GCS object versioning. Restore tested (per the maintenance SLA).
- Scaling path (when needed): the same stateless image scales on Cloud Run; add Cloud SQL read replicas for dashboard/matching reads; no rewrite required.
