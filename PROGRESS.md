# PROGRESS.md — Where we are right now

> **This is the file to open to see what's done, what's happening, and what's next.** Updated and committed after every unit of work. The full task list lives in `docs/BUILD_PLAN.md`; this is the running status on top of it.

**Last updated:** 2026-06-21
**Current milestone:** M3 — Angrosist LIVE + dashboard (in progress)
**Done:** ANAF→DemoANAF repoint + CAEN→roles ✅ · dashboard scaffold (024/025 + views) ✅ · auth + RBAC + users ✅ · dashboard data API (pipeline/detail/offer/assign/companies/handoffs/kpis, cursor pagination, openapi updated) ✅
**Next:** frontend dashboard (login + pipeline + lead detail + offer UI + directory + handoff, shadcn) → then Mailer (email) + handoff_to_human tool + GCS FileStore (upload_media).
**Branches:** `main` = the **Vercel demo** (frozen at pre-today `b5d1b3d`, do not push WIP here). `develop` = **active build** (this is where we work). Push auth fixed (gh active account → `victormihaita`).

---

## Execution order (the plan I'm following)

1. ✅ **Setup** — Claude OS, hard rules, BUILD_PLAN, specs, MCP, hooks _(done & merged)_
2. ✅ **User journeys doc** — confirm the final outcome _(done)_
3. ✅ **M0.5 — Demo hardening** — config hygiene, CORS, validation, error boundary, tests _(done; H3→M2)_
4. ✅ **M1 — Foundation** — refactor done, full Phase-1 schema (009-023, tested), Terraform + CI/CD as code (dormant), local Docker. _ANAF repoint→M3; GCP provisioning + Meta verification = owner actions._
5. ✅ **M2 — Agent core + web widget** — LLM port + Gemini/Claude adapters, async runtime, SSE widget + typing
6. ⏳ **M3 — Angrosist + dashboard** — Angrosist LIVE end-to-end _(next)_
7. ⬜ **M4 — WhatsApp + PalletClearance + photos + i18n**
8. ⬜ **M5 — KPIs, GDPR/security, backup, testing, handover**
9. ⬜ **Phase 2 (M2.1–M2.3)** — SkalYou marketplace
10. ⬜ **Phase 3** — Country Operator

Legend: ✅ done · ⏳ in progress · ⬜ not started

---

## M0.5 — Demo hardening (live checklist)

> Goal: make the demo safe and modular before scaling it into Phase 1. Source: `docs/BUILD_PLAN.md` → M0.5.

### H1 — Secrets & config hygiene ✅
- [x] `.env` gitignored confirmed; test-keys decision documented (kept as-is per owner)
- [x] Gemini model name → `GEMINI_MODEL` env (no baked constant); ANAF base URL already env
- [x] Both `.env.example` files completed (model, PORT, CORS, API URL)

### H2 — API surface safety ✅
- [x] CORS replaced with env-driven, origin-aware allowlist (`CORS_ALLOWED_ORIGINS`)
- [x] Input validation on `/api/chat` (trim, length, conversation_id) + body cap; CUI already validated in adapter
- [x] Request size limit (64KB `MaxBytesReader`)  ·  _rate limiting → deferred to M2 (needs the async/edge layer)_

### H3 — LLM behind a port → **folded into M2** (by design)
- [ ] Provider-neutral `LLM` port + move Gemini into an adapter — done as part of M2 Epic 2.1 (agent-core split), so the runner is restructured once, not twice.
- [ ] Retry/backoff on LLM calls — with the port, in M2. (ANAF already has a 10s timeout; ANAF retry/fallback lands in M1 Epic 1.5 repoint.)

### H4 — Baseline tests & errors ✅
- [x] Backend unit test (CORS resolution) + fixed pre-existing broken ANAF test (implemented demo mode)
- [x] Frontend app error boundary + resilient QueryClient defaults
- [ ] Toast system (sonner) → M3 dashboard, via shadcn MCP _(full use-case test with a mock LLM port lands with the M2 port)_

---

## Changelog (newest first)

- **2026-06-21** — **M3 backend progress.** (1) ANAF→DemoANAF repoint: richer verified company data + CAEN→roles, company_verifications audit row, `ANAF_PROVIDER` selects demoanaf/anaf/demo. (2) Dashboard scaffold: migrations 024 (users.password_hash) + 025 (leads.offer_value/note) + domain view types. (3) Auth + RBAC: bcrypt + HS256 JWT, login, Require/RequireRole middleware, admin bootstrap from env, admin-only users API. (4) Dashboard data API: secured leads pipeline (cursor pagination + filters), lead detail (transcript + typed request + company/verification + contact), offer tracking + assignment (audited), B2B directory, handoff queue, basic KPIs; openapi.yaml updated + validated. All real-DB tested.
- **2026-06-21** — **M2 complete.** Epic 2.2: Queue port (local + Cloud Tasks push) + `cmd/worker` endpoint + Postgres per-conversation advisory lock + idempotency by provider message ID (real-DB tested). Epic 2.3: in-process SSE `Broker` port + `GET /api/stream` (typing/message/error, heartbeat) publishing from both sync and async paths; frontend `useChat` consumes SSE with a ~1.5s POST fallback (keeps the Vercel demo working). `/api/chat` response unchanged throughout.
- **2026-06-21** — **M2 Epic 2.1 done.** Split the coupled Gemini runner into a vendor-neutral `LLM` port + agent core (turn loop, tool registry, state machine, RO prompt) + thin Gemini adapter, then added a **Claude/Anthropic adapter** behind the same port. `LLM_PROVIDER` env swaps providers (gemini default, claude for prod); agent core has zero vendor SDK imports (verified). Unit-tested with mock LLM + the Claude mapping helpers.
- **2026-06-21** — **M1 cloud-ready (deferred provisioning).** `deploy/terraform/`: 8 modules (Artifact Registry, IAM SAs, Secret Manager, Cloud SQL, GCS, Cloud Run, Cloud Tasks, Cloud Scheduler) composed per env (dev/staging/prod); EU region, least-privilege, no secret values. `.github/workflows/`: CI runs on push; GCP deploy jobs DORMANT (workflow_dispatch + `DEPLOY_ENABLED` guard) via Workload Identity Federation. Dockerfile now builds the worker too. Validated with tofu + actionlint. **M1 buildable scope complete.**
- **2026-06-21** — **Full Phase-1 schema** (migrations 009-023): lookups, users, categories, company_verifications/financials, consents, listings, buyer_profiles, documents, activity_logs, templates; enriched companies/contacts/leads/etc.; all indexes + GDPR cascade. Tested clean apply + idempotent on scratch Postgres.
- **2026-06-21** — **Branch split.** `main` reverted to `b5d1b3d` (pre-today) so Vercel keeps serving the stable demo; all today's work moved to the new `develop` branch, which is now the active build line. `origin` updated to match.
- **2026-06-21** — **Local Docker stack** added: `docker compose up --build` runs Postgres + migrations + backend + frontend (no GCP needed). Backend/frontend Dockerfiles, nginx SPA config, compose with healthcheck + migrate-before-backend ordering, root `.env.example`. Compose config validated (daemon was down, so no build run here — run locally).
- **2026-06-21** — **Backend refactored** to the target `/cmd` + `/internal` layout (M1 Epic 1.1): pkg→internal moves via git mv, `CompanyVerifier`→`CompanyDataProvider`, `cmd/worker` skeleton. Gemini kept whole (LLM-port split is M2). Build/vet/test green.
- **2026-06-21** — Fixed git push (gh had two accounts; switched active to `victormihaita`).
- **2026-06-21** — **M0.5 complete.** Backend: env-driven CORS allowlist, `/api/chat` input validation + 64KB body cap, `GEMINI_MODEL` env, implemented ANAF demo mode (fixed a broken test), gofmt-normalized, CORS unit test. Frontend: app error boundary, resilient QueryClient defaults, lint+build clean. H3 (LLM port) intentionally folded into M2.
- **2026-06-21** — Audited the company-verification provider → `docs/specs/ANAF_API.md`. Decision: use DemoANAF **free** REST API (richer than the raw ANAF service the demo currently calls); endpoints `/company/:cui` (+ optional `/financials`, `/caen`, `/search`); free tier is enough; flagged a demo/adapter discrepancy + missing commercial terms. Threaded tasks into BUILD_PLAN M1 Epic 1.5.
- **2026-06-21** — Added `docs/USER_JOURNEYS.md` (all actor journeys) + this tracker. Starting M0.5.
- **2026-06-21** — Merged Claude OS + planning suite to `main` (hard rules, BUILD_PLAN, 6 specs, 7 agents, 6 commands, MCP, hooks).
- **2026-06-21** — Wired Context7 + shadcn MCP (key in git-ignored local settings).
