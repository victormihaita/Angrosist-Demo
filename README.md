# Euro Intermed B2B Platform

An AI-powered lead-qualification and matching platform for Romanian wholesale/B2B commerce. An AI agent talks to prospects on multiple channels (embeddable web widget, WhatsApp, and more), qualifies them into structured leads, verifies their company live against the Romanian registry, and a consultant works those leads in a dashboard. The strategic asset is a **normalized B2B company database** that later powers expert matching.

**Verticals:** Angrosist (wholesale buyers) and PalletClearance (buyers + sellers-with-photos) in Phase 1; SkalYou (diagnostic → expert matching) in Phase 2. **Languages:** RO/EN. Romania first, built to extend.

> ⚠️ The repo currently contains a **Milestone-0 demo** (the slice that's live). The full product is specified under `docs/` and planned in `docs/BUILD_PLAN.md`.

## Documentation map

| Doc | Purpose |
|---|---|
| `CLAUDE.md` | Project rules, fixed stack, **Hard rules (non-negotiable)** |
| `docs/PRODUCT.md` | What we're building, for whom, scope boundaries |
| `docs/REQUIREMENTS.md` | Functional + non-functional requirements (phase-tagged) |
| `docs/ARCHITECTURE.md` | Hexagonal architecture, async agent runtime, GCP topology |
| `docs/DATA_MODEL.md` | Entities, enums, invariants |
| `docs/DEVELOPMENT_PLAN.md` | Milestones, commercials, acceptance criteria |
| `docs/BUILD_PLAN.md` | **Execution source of truth** — milestone → epic → task |
| `docs/specs/ARCHITECTURE_DETAIL.md` | Concrete port interfaces + the adapter "swap recipe" |
| `docs/specs/DATA_MODEL_DDL.md` | Full PostgreSQL DDL, relations, indexes, GDPR cascade |
| `docs/specs/API_CONTRACT.md` + `openapi.yaml` | REST/WS contract |
| `docs/specs/SECURITY.md` | Threat model, RBAC, secrets, GDPR, audit catalog |
| `docs/specs/AI_AGENT_SPEC.md` | Agent design, prompt library, tool schemas, guardrails |
| `docs/specs/UIUX_GUIDE.md` | shadcn component inventory per screen |
| `CONTRIBUTING.md` | Workflow, conventions, the modular-swap recipe |

## Architecture at a glance

- **Hexagonal / ports & adapters.** Domain logic imports no infrastructure. Every external concern (LLM, channel, DemoANAF, GCS, email, queue, DB) is a port with one swappable adapter. See `docs/specs/ARCHITECTURE_DETAIL.md`.
- **Channel-agnostic, multi-interface agent.** One flow engine + LLM port serves the web widget, WhatsApp, and future channels.
- **Async agent turns.** Ack fast → enqueue (Cloud Tasks) → stateless worker runs the LLM/tools. Idempotent, per-conversation locked.
- **GCP-native** (production), Terraform-defined. EU data residency required.

## Repository layout

```
backend/     # Go service (API, agent core, adapters, migrations). Independently deployable.
frontend/    # React + TypeScript (dashboard + embeddable widget). Independently deployable.
docs/        # Product/spec docs + BUILD_PLAN + specs/
.claude/     # Claude tooling: agents, commands, hooks, settings
.mcp.json    # MCP servers (shadcn/ui, Context7)
```

## Tech stack (locked)

Go backend (Docker/Cloud Run) · PostgreSQL (Cloud SQL) · Cloud Tasks · Cloud Scheduler · GCS · Secret Manager · Artifact Registry · React/TypeScript frontend · Anthropic Claude (LLM, prod) / Gemini (demo) · WhatsApp Cloud API · DemoANAF (company verification) · external EU email provider · Cloudflare · Terraform · Cloud Logging/Monitoring + Sentry.

## Run the whole stack locally with Docker (no GCP needed)

The fastest way to test end-to-end. Brings up Postgres + runs migrations + backend API + frontend:

```bash
cp backend/.env.example backend/.env   # add your GEMINI_API_KEY (the DB URL is set by compose)
docker compose up --build
```

Then open:
- Frontend → http://localhost:5173
- Backend health → http://localhost:8080/api/health

Notes:
- The local Postgres connection is injected by `docker-compose.yml` and **overrides** `DATABASE_URL` in `backend/.env`, so the stack is self-contained (no Neon needed locally).
- Secrets come from `backend/.env` (gitignored). Compose-level knobs (DB creds, CORS, `VITE_API_URL`) can be overridden via a root `.env` (see `.env.example`).
- To run the agent without any external ANAF dependency, set `ANAF_DEMO_MODE=true` in `backend/.env`.
- Stop with `docker compose down` (add `-v` to wipe the database volume).

## Local development (without Docker)

> Detailed setup lives in `CONTRIBUTING.md`. Quick version:

**Backend** (`backend/`):
```bash
cp .env.example .env      # fill in values (never commit .env)
go run ./cmd/migrate      # run migrations
go run ./cmd/server       # start the API (PORT default 8080)
```

**Frontend** (`frontend/`):
```bash
cp .env.example .env      # set VITE_API_URL to the backend URL
npm install
npm run dev               # app
npm run build:widget      # build the embeddable widget
```

## Configuration & secrets

**Nothing is hardcoded.** All config comes from environment variables; all secrets come from Secret Manager (prod) or a gitignored `.env` (local). See the secret inventory in `docs/specs/SECURITY.md`. A pre-commit hook blocks commits that look like they contain a secret.

## Status & roadmap

See `docs/BUILD_PLAN.md` for the live milestone status (M0 demo built; M0.5 hardening → M1 foundation → M5 handover; then Phase 2/3).
