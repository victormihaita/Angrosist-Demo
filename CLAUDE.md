# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

Euro Intermed B2B Platform — an AI-powered qualification and matching platform for Romanian wholesale/B2B buyers. It has two verticals: **Angrosist** (wholesale buyer qualification) and **PalletClearance** (pallet lot buy/sell), with **SkalYou** (market-entry matchmaking) as Phase 2.

The full specification is split across companion docs (read in order when needed):
- `docs/PRODUCT.md` — what we're building, for whom, scope boundaries
- `docs/REQUIREMENTS.md` — functional + non-functional requirements, phase-tagged
- `docs/ARCHITECTURE.md` — hexagonal architecture, async agent runtime, GCP topology
- `docs/DATA_MODEL.md` — entities, enums, invariants
- `docs/DEVELOPMENT_PLAN.md` — milestones, deliverables, acceptance criteria

**Execution & handover docs (build against these):**
- `docs/BUILD_PLAN.md` — milestone → epic → task breakdown with acceptance criteria (the execution source of truth)
- `docs/specs/ARCHITECTURE_DETAIL.md` — concrete port interfaces + the adapter "swap recipe"
- `docs/specs/DATA_MODEL_DDL.md` — full PostgreSQL DDL, relations, indexes, GDPR cascade
- `docs/specs/API_CONTRACT.md` + `docs/specs/openapi.yaml` — the REST/WS contract
- `docs/specs/SECURITY.md` — threat model, RBAC, secrets, GDPR procedures, audit catalog
- `docs/specs/AI_AGENT_SPEC.md` — agent design, versioned prompt library, tool schemas, guardrails
- `docs/specs/UIUX_GUIDE.md` — shadcn component inventory per screen
- `backend/CLAUDE.md`, `frontend/CLAUDE.md` — layer-specific conventions
- `CONTRIBUTING.md` — workflow, branch/PR/test conventions, the modular-swap recipe

## Fixed stack decisions

These are locked — do not propose alternatives:

| Layer | Technology |
|---|---|
| Backend | Go (Docker image on Cloud Run) |
| Frontend | React / TypeScript |
| Database | Cloud SQL (PostgreSQL) |
| LLM | Claude (Anthropic) — Phase 1+; **Gemini (`gemini-2.0-flash-lite`) for Milestone 0 demo only** |
| Messaging | WhatsApp Cloud API |
| Company verification | DemoANAF `GET /api/company/:cui` |
| File storage | GCS |
| Queue / async | Cloud Tasks |
| Cron | Cloud Scheduler |
| Secrets | Secret Manager |
| Container registry | Artifact Registry |
| Frontend hosting | Firebase Hosting or Cloud Run |
| DNS / edge | Cloudflare |
| IaC | Terraform |

All infrastructure runs on **GCP**. EU data residency is required.

## Architecture principles

- **Hexagonal (clean) architecture** — domain logic is isolated from adapters (HTTP, WhatsApp, DB, LLM). The agent core has no import of infrastructure packages.
- **Channel-agnostic agent** — the same flow engine and LLM port powers both the web widget and WhatsApp. Channel-specific code lives only in adapters.
- **Async agent turns** — conversations are driven via Cloud Tasks workers. Each turn is idempotent (deduplication key on message ID). Per-conversation lock prevents concurrent turn processing.
- **Stateless services** — Cloud Run instances carry no in-memory state; all state lives in Cloud SQL.
- **Additive data model** — schema migrations must not reshape existing `companies`, `contacts`, `leads`, or `sourcing_requests` data. Add columns/tables; avoid drops or renames on existing fields.
- **Build inside-out** — per milestone: domain layer → adapters → API → UI.

## Hard rules (non-negotiable)

These override convenience. A change that violates one of these is wrong even if it "works".

1. **No hardcoded secrets or URLs — ever.** Every secret and every external URL/base-path comes from environment variables (local: gitignored `.env`; prod: Secret Manager). No keys, passwords, connection strings, endpoints, or model names baked into source. A pre-commit hook scans for this; do not bypass it.
2. **Ports & adapters for all I/O.** Domain/use-case code imports zero infrastructure. Every external concern (LLM, channel, DemoANAF, GCS, email, queue, DB) is a port interface with exactly one production adapter and a mock for tests. Swapping a provider = new adapter, zero domain changes. See `docs/specs/ARCHITECTURE_DETAIL.md`.
3. **The agent is channel-agnostic and multi-interface.** The same flow engine + LLM port serves the web widget, WhatsApp, and any future channel (Telegram, Instagram, voice, …). Channel-specific code lives only in a `Channel` adapter. Adding an interface must not touch the agent core.
4. **The LLM never holds keys or touches data directly.** It only emits tool calls (`verify_company`, `upload_media`, `submit_lead`, `handoff_to_human`, P2 `classify_need`); our code validates the args and executes them against ports.
5. **Migrations are forward-only and additive.** Never drop/rename/reshape existing `companies`, `contacts`, `leads`, `sourcing_requests` data. New verticals/fields = new sibling tables / nullable columns.
6. **Validate every external input.** Chat text, CUI, file type/size, email, phone, webhook payloads. Parameterized SQL only. Treat LLM output and user content as untrusted (prompt-injection aware).
7. **Async by default for agent turns.** Ack the webhook/WS fast, enqueue (Cloud Tasks), let a stateless worker run the LLM/tools. Dedupe by provider message ID; take a per-conversation lock.
8. **Structured logging, correlation IDs, no PII in logs.** Errors wrapped with context; distinguish retryable vs terminal for workers.
9. **Security & GDPR are first-class, not a milestone afterthought.** Consent capture, audit log of meaningful actions, cascade erasure, RBAC, EU data residency, signed webhooks. See `docs/specs/SECURITY.md`.
10. **Backend and frontend stay independently deployable.** No shared build coupling; the frontend talks to the backend only over the documented API; base URL via `VITE_*`.
11. **Frontend uses shadcn/ui prebuilt components as-is.** No bespoke component CSS forking; prioritize performance and accessibility. See `docs/specs/UIUX_GUIDE.md`.
12. **Document what you build.** Public Go symbols have godoc; new endpoints update `openapi.yaml`; new packages get a short README. The project must stay handover-ready.
13. **Tests gate "done".** Critical paths (qualification, verification, lead creation, handoff, erasure) covered before a milestone is complete.
14. **Respect phase boundaries.** Don't build Phase-2 features in Phase 1 — but never break the data model's ability to allow them.

## Monorepo structure

This is a monorepo with two independent Vercel project roots:

| Folder | Vercel project | Purpose |
|---|---|---|
| `backend/` | Backend project (Go runtime) | API handlers, domain, adapters, migrations |
| `frontend/` | Frontend project (Vite) | React app, chat widget |

- `backend/go.mod` is the Go module root (`github.com/angrosist/demo`)
- `frontend/package.json` is the Node root
- A single `.gitignore` at repo root covers both
- `frontend/VITE_API_URL` env var points to the backend Vercel URL

## Current phase: Milestone 0 (demo)

Milestone 0 delivers a thin vertical slice:
- Hosted web chat (Angrosist buyer, RO)
- Agent qualifies via conversation (product, quantity, location, CUI extraction) using Gemini 2.5 Flash
- Live CUI verification via DemoANAF
- Writes to schema: `companies`, `contacts`, `leads`, `sourcing_requests`
- Dashboard: lead list + transcript + extracted fields + embeddable widget code

**Deploy for demo:** Vercel (backend + frontend as separate projects) + Neon PostgreSQL; production is GCP.

## Key invariants

- WhatsApp channel is gated on Meta Business verification (start early, run in parallel with M1).
- The B2B directory + `roles[]` company tagging must ship in Phase 1 — Phase 2 matching depends on it.
- GDPR: consent, retention, cascade erasure, and audit log are Phase 1 (M5) requirements, not optional.
- Clickwrap (timestamp/IP/version recorded) is required before any client data is shown to a provider (Phase 2).
- Seller photo upload in PalletClearance blocks conversation progress — the flow must not advance without photos.

## Claude tooling in this repo

This repo ships a Claude "operating system" to make the build disciplined and fast:

- **Subagents** (`.claude/agents/`) — `go-backend-architect`, `db-migration-engineer`, `agent-prompt-engineer`, `frontend-shadcn-builder`, `security-gdpr-auditor`, `infra-terraform-engineer`, `api-contract-keeper`. Delegate specialized work to these.
- **Slash commands** (`.claude/commands/`) — `/new-adapter`, `/new-migration`, `/new-vertical`, `/agent-eval`, `/secret-scan`, `/milestone-check`.
- **MCP servers** (`.mcp.json`) — shadcn/ui (pull prebuilt components) and Context7 (current library docs). Config from env, never inlined.
- **Hooks & settings** (`.claude/settings.json`) — pre-commit secret/URL scan, formatting on save, a read-only Bash allowlist.

When work matches an agent or command, prefer it over ad-hoc steps.

## CI/CD expectations

- CI builds Docker image → pushes to Artifact Registry → deploys to Cloud Run (staging).
- DB migrations run as part of deploy, not manually.
- Secrets are injected from Secret Manager at runtime — never hardcoded or in env files committed to the repo.
