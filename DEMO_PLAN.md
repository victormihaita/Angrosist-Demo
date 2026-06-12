# DEMO_PLAN.md — Milestone 0: Demo Build Plan

Single vertical (Angrosist buyer, RO), deployed on **Vercel + Neon**. This becomes Phase 1's first slice — nothing here is throwaway.

> **Progress tracking:** ⬜ Not started · 🔄 In progress · ✅ Done

---

## Stack (demo)

| Layer | Technology |
|---|---|
| Frontend | React + TypeScript (Vite), Tailwind CSS, shadcn/ui |
| Backend | Go serverless functions (`/api/*.go` on Vercel) |
| Database | Neon (serverless PostgreSQL) |
| LLM | Gemini API (`gemini-2.5-flash`) |
| Company verification | DemoANAF `GET /api/company/:cui` |
| Deploy | Vercel (monorepo — Go functions + React frontend) |

---

## Repository structure

Monorepo with two independent Vercel project roots: `backend/` and `frontend/`.

```
/                                     # repo root
├── backend/                          # Vercel project root (Go)
│   ├── api/                          # Vercel Go serverless handlers
│   │   ├── health.go                 # GET /api/health
│   │   ├── chat.go                   # POST /api/chat
│   │   ├── leads.go                  # GET /api/leads
│   │   └── leads/
│   │       └── [id].go               # GET /api/leads/:id
│   ├── internal/
│   │   ├── domain/                   # Pure domain types — zero external imports
│   │   │   ├── company.go
│   │   │   ├── conversation.go
│   │   │   ├── lead.go
│   │   │   └── sourcing.go
│   │   ├── ports/                    # Go interfaces (hexagonal ports)
│   │   │   ├── repositories.go
│   │   │   └── services.go
│   │   ├── usecases/                 # Application use cases
│   │   │   ├── chat.go
│   │   │   └── leads.go
│   │   ├── adapters/
│   │   │   ├── postgres/             # DB repository implementations
│   │   │   │   ├── db.go             # pgx pool singleton
│   │   │   │   ├── company.go
│   │   │   │   ├── conversation.go
│   │   │   │   ├── message.go
│   │   │   │   ├── lead.go
│   │   │   │   └── sourcing.go
│   │   │   ├── gemini/               # Gemini LLM adapter
│   │   │   │   ├── client.go
│   │   │   │   ├── prompt.go
│   │   │   │   ├── tools.go
│   │   │   │   └── runner.go
│   │   │   ├── anaf/                 # DemoANAF HTTP adapter
│   │   │   │   └── client.go
│   │   │   └── http/                 # Shared HTTP helpers
│   │   │       └── response.go
│   │   └── app/                      # Dependency wiring
│   │       └── container.go
│   ├── cmd/
│   │   └── migrate/
│   │       └── main.go               # Migration runner
│   ├── migrations/                   # SQL migration files
│   │   ├── 001_create_companies.sql
│   │   ├── 002_create_conversations.sql
│   │   ├── 003_create_messages.sql
│   │   ├── 004_create_contacts.sql
│   │   ├── 005_create_leads.sql
│   │   └── 006_create_sourcing_requests.sql
│   ├── go.mod
│   ├── go.sum
│   ├── .env                          # git-ignored
│   └── .env.example
│
├── frontend/                         # Vercel project root (React)
│   ├── src/
│   │   ├── components/
│   │   │   ├── ui/                   # shadcn/ui generated components
│   │   │   ├── chat/
│   │   │   │   ├── MessageList.tsx
│   │   │   │   ├── MessageInput.tsx
│   │   │   │   └── ExtractionStatus.tsx
│   │   │   ├── dashboard/
│   │   │   │   ├── LeadTable.tsx
│   │   │   │   ├── LeadDetail.tsx
│   │   │   │   └── EmbedDialog.tsx
│   │   │   └── layout/
│   │   │       └── Nav.tsx
│   │   ├── pages/
│   │   │   ├── ChatPage.tsx
│   │   │   ├── DashboardPage.tsx
│   │   │   └── LeadDetailPage.tsx
│   │   ├── hooks/
│   │   │   ├── useLeads.ts
│   │   │   └── useLead.ts
│   │   ├── lib/
│   │   │   ├── api.ts
│   │   │   └── utils.ts
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── widget/
│   │   ├── WidgetApp.tsx             # Self-contained floating chat panel
│   │   └── widget-entry.tsx          # IIFE entry → window.AngrosistChat.init()
│   ├── vite.config.ts                # Main app build → dist/
│   ├── vite.widget.config.ts         # Widget IIFE build → dist-widget/widget.js
│   ├── .env                          # git-ignored
│   ├── .env.example
│   └── package.json
│
├── .gitignore                        # covers both backend/ and frontend/
└── README.md
```

**Vercel setup (two separate projects):**
- Backend project → root directory: `backend/` (Go runtime, handles `/api/*`)
- Frontend project → root directory: `frontend/` (Vite build)
- Frontend `VITE_API_URL` env var points to the backend Vercel project URL

---

## ✅ Phase 1 — Project Scaffolding & Env *(done)*

**Goal:** Runnable skeleton — Go module, deps, env files, Vite frontend, health endpoint.

### Tasks
- [x] `go mod init github.com/angrosist/demo` + add all Go deps
- [x] `.env` (real values, git-ignored) + `.env.example` (committed template)
- [x] `.gitignore` — ignores `.env`, `frontend/.env`
- [x] `vercel.json` — routes `/api/*` → Go runtime, `/*` → `frontend/dist`
- [x] `internal/adapters/postgres/db.go` — `pgx.Pool` singleton via `sync.Once`
- [x] `internal/app/container.go` — global DI container skeleton (`sync.Once`, `godotenv.Load`)
- [x] `api/health.go` — `GET /api/health` → `{"ok":true,"db":true}`
- [x] `frontend/` — Vite + React + TypeScript scaffolded
- [x] `frontend/.env` + `frontend/.env.example`

**AC:** `go build ./...` succeeds. Health endpoint responds 200.

---

## ✅ Phase 2 — Database Schema & Migrations *(done — all 6 tables applied to Neon)*

**Goal:** All 6 migration files + idempotent migration runner.

### Tasks
- [x] `migrations/001_create_companies.sql`
- [x] `migrations/002_create_conversations.sql`
- [x] `migrations/003_create_messages.sql`
- [x] `migrations/004_create_contacts.sql`
- [x] `migrations/005_create_leads.sql`
- [x] `migrations/006_create_sourcing_requests.sql`
- [x] `cmd/migrate/main.go` — reads files in order, tracks in `schema_migrations`, idempotent

**Run with:** `DATABASE_URL=<neon-url> go run ./cmd/migrate`

**AC:** Runner creates all 6 tables. Re-running is a no-op.

---

## ✅ Phase 3 — Domain, Ports & Adapters *(done)*

**Goal:** All backend business logic, no API wiring yet.

### Tasks
- [ ] `internal/domain/` — pure Go structs for all entities
- [ ] `internal/ports/repositories.go` — 5 repository interfaces
- [ ] `internal/ports/services.go` — `CompanyVerifier` interface
- [ ] `internal/adapters/postgres/` — implement all 5 repos with raw pgx
- [ ] `internal/adapters/anaf/client.go` — DemoANAF HTTP adapter
- [ ] `internal/adapters/gemini/client.go` — `genai.Client` singleton, model `gemini-2.5-flash`
- [ ] `internal/adapters/gemini/prompt.go` — Romanian system prompt
- [ ] `internal/adapters/gemini/tools.go` — `verify_company` + `save_lead` function declarations
- [ ] `internal/adapters/gemini/runner.go` — function-calling loop (load history → SendMessage → execute tools → persist → return text)

**AC:** `go build ./...` clean. ANAF adapter returns company data for a valid CUI.

---

## ✅ Phase 4 — Use Cases & API Handlers *(done — go build ./... clean)*

**Goal:** Endpoints wired end-to-end through the clean architecture.

### Tasks
- [ ] `internal/usecases/chat.go` — `ChatUseCase.RunTurn()`
- [ ] `internal/usecases/leads.go` — `LeadUseCase.List()` + `GetByID()`
- [ ] `internal/app/container.go` — wire all adapters → use cases
- [ ] `internal/adapters/http/response.go` — `WriteJSON`, `WriteError`, CORS helper
- [ ] `api/chat.go` — POST handler
- [ ] `api/leads.go` — GET list handler
- [ ] `api/leads/[id].go` — GET detail handler

**AC:** Full conversation via `curl` reaches `state:"confirmed"`. Lead visible in `GET /api/leads`.

---

## ✅ Phase 5 — Frontend: Chat Page + Embeddable Widget *(done — both builds pass)*

**Goal:** Full-page chat + a separately bundled embeddable widget.

### Tasks
- [ ] shadcn init + install components (`button`, `input`, `textarea`, `badge`, `card`, `scroll-area`, `separator`, `dialog`)
- [ ] `src/lib/api.ts` — typed fetch client (sendMessage, getLeads, getLead)
- [ ] `src/components/chat/MessageList.tsx` — scrollable bubbles, auto-scroll
- [ ] `src/components/chat/MessageInput.tsx` — textarea + send, Enter submits
- [ ] `src/components/chat/ExtractionStatus.tsx` — 5-field checklist panel
- [ ] `src/pages/ChatPage.tsx` — full-page layout, session-persisted conversation_id
- [ ] `frontend/widget/WidgetApp.tsx` — compact floating panel (same logic, fixed bottom-right)
- [ ] `frontend/widget/widget-entry.tsx` — IIFE entry, `window.AngrosistChat.init(config)`
- [ ] `frontend/vite.widget.config.ts` — lib IIFE build, CSS injected into bundle
- [ ] `package.json` scripts: `build`, `build:widget`, `build:all`

**Embed snippet (shown in dashboard):**
```html
<script src="https://YOUR_DOMAIN/widget.js"></script>
<script>AngrosistChat.init({ apiUrl: 'https://YOUR_DOMAIN' });</script>
```

**AC:** Chat conversation completes on `/`. `npm run build:widget` produces `dist-widget/widget.js`. Embedding the snippet on a blank page opens a functional floating chat.

---

## ✅ Phase 6 — Frontend: Dashboard *(done)*

**Goal:** Admin view with lead list, lead detail, and widget embed code dialog.

### Tasks
- [ ] `src/components/layout/Nav.tsx` — top bar with Chat / Dashboard links + "Embed Widget" button
- [ ] `src/components/dashboard/EmbedDialog.tsx` — shadcn Dialog with copyable snippet
- [ ] `src/components/dashboard/LeadTable.tsx` — shadcn Table, TanStack Query, 30s refresh
- [ ] `src/pages/DashboardPage.tsx` — `/dashboard` route
- [ ] `src/components/dashboard/LeadDetail.tsx` — two-column: fields card + transcript
- [ ] `src/pages/LeadDetailPage.tsx` — `/dashboard/:id` route
- [ ] `src/hooks/useLeads.ts` + `useLead.ts` — TanStack Query hooks
- [ ] `src/App.tsx` — router + QueryClientProvider

**Status badge colours:** `new`=blue · `qualifying`=yellow · `confirmed`=green · `failed`=red

**AC:** Dashboard lists all leads. Row click shows full transcript + extracted fields. EmbedDialog shows copyable code.

---

## ✅ Phase 7 — Integration & Deploy *(local integration complete)*

**Goal:** Demo-ready shareable link.

### Tasks
- [x] End-to-end: chat → lead in dashboard with transcript (tested locally)
- [x] ANAF adapter fixed: tries real API, falls back to demo data (`ANAF_DEMO_MODE=true` in `.env`)
- [x] `GET /api/leads` returns saved leads; `GET /api/leads/:id` returns transcript
- [x] `go build ./...` clean; `npm run build:all` produces `dist/` + `dist-widget/widget.js`
- [x] Local dev server at `cmd/server/main.go` (port 8080, mirrors Vercel routing)
- [ ] Vercel deploy: set `DATABASE_URL`, `GEMINI_API_KEY`, `ANAF_DEMO_MODE=true` in Vercel dashboard
- [ ] Set frontend `VITE_API_URL` to backend Vercel URL, redeploy frontend
- [ ] Smoke test on prod URL: `/api/health`, full chat, dashboard

**Demo script:**
> Open `/` → type "vreau să cumpăr ulei de floarea-soarelui, 5000 kg, livrare în Cluj" → agent asks for CUI → provide valid CUI → agent verifies company → lead saved → visible in `/dashboard`

---

## What this demo intentionally excludes

Phase 1 concerns — not built here:
- Authentication on the dashboard
- WhatsApp channel
- Email notifications
- Document upload
- PalletClearance / SkalYou verticals
- GDPR / consent flows
- Rate limiting, Terraform, GCP infrastructure
