# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

Euro Intermed B2B Platform — an AI-powered qualification and matching platform for Romanian wholesale/B2B buyers. It has two verticals: **Angrosist** (wholesale buyer qualification) and **PalletClearance** (pallet lot buy/sell), with **SkalYou** (market-entry matchmaking) as Phase 2.

The full specification is split across companion docs (read in order when needed):
- `PRODUCT.md` — what we're building, for whom, scope boundaries
- `REQUIREMENTS.md` — functional + non-functional requirements, phase-tagged
- `ARCHITECTURE.md` — hexagonal architecture, async agent runtime, GCP topology
- `DATA_MODEL.md` — entities, enums, invariants
- `DEVELOPMENT_PLAN.md` — milestones, deliverables, acceptance criteria

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

## CI/CD expectations

- CI builds Docker image → pushes to Artifact Registry → deploys to Cloud Run (staging).
- DB migrations run as part of deploy, not manually.
- Secrets are injected from Secret Manager at runtime — never hardcoded or in env files committed to the repo.
