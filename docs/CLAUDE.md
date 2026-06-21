# CLAUDE.md — Project Rules & Conventions

> Read this first. It is the entry point for working on this codebase.
> Companion specs: `PRODUCT.md`, `REQUIREMENTS.md`, `ARCHITECTURE.md`, `DATA_MODEL.md`, `DEVELOPMENT_PLAN.md`.

## 1. What this is

A B2B lead-qualification platform for **Euro Intermed**. An AI agent talks to prospects on two channels (an embeddable web chat widget and WhatsApp), qualifies them into structured leads, and a consultant works those leads in a dashboard. Verticals: **Angrosist** (buyers) and **PalletClearance** (buyers + sellers-with-photos) in Phase 1; **SkalYou** (diagnostic → expert matching) in Phase 2. Languages: RO/EN. Romania first, built to extend.

The strategic asset is the **normalized B2B company database** built from every qualified conversation; it later powers SkalYou matching. Protect that asset: keep data in our own Postgres, never in a third-party silo.

## 2. Tech stack (authoritative — do not substitute without updating this file)

- **Backend:** Go. Single backend service, packaged as a **Docker image**.
- **Database:** PostgreSQL on **Cloud SQL**.
- **Cache / locks / dedupe:** Redis on **Memorystore** (optional in early milestones; Postgres-only is acceptable until needed).
- **Async queue:** **Cloud Tasks** (push to a worker endpoint). Pub/Sub only if fan-out is needed.
- **Cron / scheduled jobs:** **Cloud Scheduler**.
- **Object storage (photos, documents):** **Cloud Storage (GCS)**, signed URLs.
- **Secrets:** **Secret Manager**. Never commit secrets; never read them from anywhere else.
- **Container registry:** **Artifact Registry**.
- **Frontend** (dashboard, web widget, Phase-2 provider portal, client offer page): React + TypeScript, deployed on GCP (**Firebase Hosting** for static SPA, or **Cloud Run** if SSR is needed).
- **LLM:** Anthropic Claude via API (provider abstracted behind a port — see ARCHITECTURE.md).
- **WhatsApp:** Meta WhatsApp Cloud API.
- **Company verification:** DemoANAF public API (`https://demoanaf.ro`).
- **Transactional email:** an external provider (SendGrid / Mailgun / Brevo). This and the LLM/WhatsApp/DemoANAF are the only non-GCP runtime dependencies.
- **CI/CD:** GitHub Actions (or Cloud Build) → Artifact Registry → deploy.
- **IaC:** Terraform for all GCP resources.
- **Observability:** Cloud Logging + Cloud Monitoring + Sentry.

**Everything runs on Google Cloud Platform.** The backend is a Docker image; all infra is GCP-native and defined in Terraform.

## 3. Architecture rules (see ARCHITECTURE.md for detail)

- **Hexagonal / ports & adapters.** Domain logic (entities, use-cases) has zero dependency on frameworks, HTTP, the LLM, or any vendor. Everything external is a port with an adapter.
- **The agent core is channel-agnostic.** WhatsApp and the web widget are adapters that normalize into one conversation model. Build qualification logic once.
- **The LLM never touches data or external services directly.** It requests actions through **tools**; our code executes them (`verify_company`, `upload_media`, `submit_lead`, `handoff_to_human`, and Phase 2 `classify_need`).
- **Async by default for agent turns.** Ingest → ack fast → enqueue (Cloud Tasks) → worker runs the LLM/tools → reply. Never block a webhook on an LLM call.
- **12-factor & stateless services.** All durable state in Cloud SQL / GCS (and Redis for hot/ephemeral). No local disk state. A redeploy must lose nothing.
- **Idempotency everywhere it matters.** Dedupe inbound messages by provider message ID. Take a per-conversation lock so two events don't process concurrently.

## 4. Repository structure (target)

```
/cmd            # entrypoints (api server, worker, migrate)
/internal
  /domain       # entities + use-cases (no framework imports)
  /agent        # channel-agnostic agent: flow engine, LLM port, tools
  /channels     # adapters: whatsapp/, webwidget/
  /verification # DemoANAF adapter (port: CompanyDataProvider)
  /persistence  # Postgres repositories (port: repositories)
  /storage      # GCS adapter (port: FileStore)
  /email        # email provider adapter (port: Mailer)
  /queue        # Cloud Tasks adapter (port: Queue)
  /api          # HTTP handlers: webhook, widget WS/SSE, dashboard API
  /config       # env + Secret Manager loading
/migrations     # SQL migrations
/deploy         # Terraform, Dockerfile, CI workflows
/web            # frontend (dashboard, widget, [P2] provider portal, client page)
```

## 5. Coding conventions

- Idiomatic Go; small packages by domain concern, not by layer-name.
- Errors: wrap with context (`fmt.Errorf("...: %w", err)`); never swallow. Distinguish retryable (5xx-ish) vs terminal (4xx-ish) for queue workers.
- All external calls (LLM, DemoANAF, WhatsApp, email) go through a port interface with a single adapter — mock the port in tests.
- Migrations are forward-only and additive where possible (the data model is designed so new verticals/fields are additive — see DATA_MODEL.md). Never write a migration that drops or rewrites existing lead/company data.
- Structured logging (JSON) with a request/conversation correlation ID. No PII in logs beyond what's necessary.
- Tests: unit-test domain use-cases and the flow engine; integration-test repositories against a real Postgres; contract-test each external adapter. Aim for the critical paths (qualification, verification, lead creation, handoff) to be covered before a milestone is "done".

## 6. Secrets, config & environments

- Three environments: **development**, **staging**, **production**. Never test on production. Deploys happen from the repo/CI, not from a laptop.
- All config via environment variables; all secrets via **Secret Manager** (DB creds, LLM key, WhatsApp token + app secret + verify token, email key). Nothing secret in the repo or in env files committed to git.
- Each environment has its own GCP project (or clearly separated resources) and its own secrets.

## 7. Security & GDPR (non-negotiable — see REQUIREMENTS.md §NFR)

- Consent captured at first contact. Separate company data (public) from personal data (personal).
- Right-to-erasure must cascade across contact → lead → conversation → files.
- Role-based access; audit log of meaningful actions. EU data residency (GCP `europe-*` regions, EU email region).
- Webhooks: verify `X-Hub-Signature-256` (WhatsApp) on every request. Magic-links (Phase 2 client) expire and are access-logged.

## 8. How to work

- **Milestone-driven.** Follow `DEVELOPMENT_PLAN.md`. Each milestone has explicit deliverables and acceptance criteria; don't drift past the milestone's scope.
- **Respect phase boundaries.** Phase-2 (SkalYou) features are out of scope for Phase 1 — but the data model must already *allow* them (it does; don't undo that). Maintenance scope ≠ new features.
- Start each milestone by confirming the data model and the relevant ports, then build inside-out (domain → adapters → API → UI).
- Keep `DATA_MODEL.md` and `ARCHITECTURE.md` updated when reality changes.

## 9. Key invariants (do not violate)

- `companies` dedup key is **(country, reg_no)** — supports foreign companies, not just Romanian CUI.
- `vertical` and `intent` are **extensible enums** — never hardcode "exactly two verticals".
- Company verification / financials are **optional/nullable** (foreign companies won't have them).
- `companies.roles[]` classification is populated opportunistically — it seeds the Phase-2 provider directory.
- A `lead` is a **thin event** pointing at a typed request (`sourcing_request` | `listing` | Phase-2 `market_entry_request`). New verticals = new sibling table, additive.
- Files (photos, offers) live in **GCS**, never on instance disk.
