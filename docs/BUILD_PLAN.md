# BUILD_PLAN.md — Execution source of truth

> Milestone → epic → task breakdown for the Euro Intermed B2B platform. Derived from `DEVELOPMENT_PLAN.md` (commercials & high-level milestones) and `REQUIREMENTS.md` (FR/NFR), made task-level and checkable.
>
> **How to use:** work milestone by milestone. A task is "done" only when its acceptance criteria pass AND the relevant hard rules (root `CLAUDE.md`) hold. Verify a milestone with `/milestone-check`.
>
> **Legend:** `[P1]` Phase 1 · `[P2]` Phase 2 · `[P1-demo]` Milestone 0. Each task notes its **port/adapter**, **dependencies**, and **security/GDPR** concerns where relevant.

---

## Status snapshot

| Milestone | State | Headline outcome |
|---|---|---|
| M0 — Demo | ✅ | Angrosist qualify → ANAF verify → lead in dashboard |
| M0.5 — Hardening | ✅ | Config hygiene, CORS, validation, LLM-port prep, error boundary |
| M1 — Foundation | ✅ | `/cmd`+`/internal` refactor, full schema, local Docker, Terraform + CI/CD as code |
| M2 — Agent core + widget | ✅ | Channel-agnostic agent, LLM port (Gemini+Claude), async runtime, SSE widget |
| M3 — Angrosist + dashboard | ✅ | **Angrosist LIVE** end-to-end + full dashboard, rich DemoANAF, email, handoff, upload |
| M4 — WhatsApp + PalletClearance | ✅ buildable | Both verticals + seller-photo gate; WhatsApp built (live = Meta-gated); RO/EN |
| M5 — KPIs/GDPR/handover | ✅ buildable | GDPR cascade erasure + consent + audit, security audit+fixes, E2E, handover docs |
| Phase 2 (M2.1–M2.3) | ⬜ | SkalYou matching marketplace (schema already allows) |
| Phase 3 | ⬜ | First Country Operator |

**Phase 1 (M1–M5) is functionally complete** in the buildable scope. Owner-gated remainders (not code): GCP provisioning + GCS adapter, backup/restore drill + load test (on GCP), live WhatsApp (Meta Business verification — start early), and the pre-production security residuals in `PROGRESS.md`.

---

## M0.5 — Demo hardening (audit remediation)

> Close the gaps the audit found before the demo becomes Phase 1's foundation. Small, high-value, mostly backend.

### Epic H1 — Secrets & config hygiene 🔴
- [ ] **Rotate leaked credentials.** `backend/.env` currently holds a real Gemini API key + Neon password in the working tree. Rotate both now; confirm `.env` is gitignored (it is); scrub from history if ever committed. _Security: critical._
- [ ] Move the Gemini model name and DemoANAF base URL fully to env (no defaults baked in source).
- [ ] Audit both `.env.example` files: every variable listed, no real values.
- **DoD:** no secret/URL literal anywhere in source; `/secret-scan` clean.

### Epic H2 — API surface safety
- [ ] Lock CORS to known origins (replace `Access-Control-Allow-Origin: *`) via env-configured allowlist. _Security._
- [ ] Add input validation on `/api/chat` (message length, conversation_id format) and CUI format before any ANAF call.
- [ ] Add request size limits + basic rate limiting on public endpoints.
- **DoD:** malformed/oversized input rejected with the standard error envelope; CORS rejects unknown origins.

### Epic H3 — LLM behind a port
- [ ] Introduce the provider-neutral `LLM` port; move the current Gemini code into a `gemini` adapter implementing it (prep for the Claude adapter in M2). Port/adapter: `LLM`.
- [ ] Add retry/backoff + timeout + graceful failure messaging on LLM and ANAF calls.
- **DoD:** swapping LLM provider is an adapter change; transient failures retried, user sees a graceful message.

### Epic H4 — Baseline tests & errors
- [ ] Unit-test the qualification/extraction use-case with a mock LLM port; contract-test the ANAF adapter (extend existing tests).
- [ ] Frontend: add an error boundary + visible network-error toast.
- **DoD:** `go test ./...` green on critical paths; frontend degrades gracefully offline.

---

## M1 — Foundation & infrastructure `[P1]`

> Outcome: backend deploys to GCP staging from CI; migrations run; a CUI verify persists a company; secrets resolve from Secret Manager. Repo is on the target hexagonal layout.

### Epic 1.1 — Repository refactor to target layout
- [ ] Migrate `backend/pkg/{domain,usecases,adapters,ports}` → `internal/{domain,agent,channels,verification,persistence,storage,email,queue,api,config}`; split `cmd/{server,worker,migrate}`. Preserve behavior; do not reshape the data model. Reference: `docs/specs/ARCHITECTURE_DETAIL.md`.
- [ ] Split the demo's `gemini/runner.go` (LLM glue + flow + tool execution coupled) into: vendor-neutral agent core (flow engine + tool dispatch) and a thin LLM adapter.
- **DoD:** build/tests green on the new layout; the dependency rule holds (domain imports no infra).

### Epic 1.2 — GCP foundation via Terraform
- [ ] Terraform: three environments (dev/staging/prod) as separate GCP projects or strictly separated resources, all in `europe-*`. Agent: `infra-terraform-engineer`.
- [ ] Provision: Cloud SQL (Postgres, automated backups + PITR), Artifact Registry, Cloud Run service skeleton, Secret Manager, Cloud Tasks queue, GCS bucket, service accounts (least-privilege).
- [ ] Cloudflare DNS + TLS; email auth DNS records (SPF/DKIM/DMARC) for the sending subdomain.
- **DoD:** `terraform apply` stands up staging; resources are EU-region; SAs are least-privilege.

### Epic 1.3 — CI/CD
- [ ] GitHub Actions: build Docker image → push to Artifact Registry → deploy to Cloud Run (staging). Migrations run as a deploy step, not manually.
- [ ] Secrets fetched from Secret Manager at deploy; never printed. Add dependency + secret scanning to CI.
- **DoD:** a push to the main branch deploys staging from CI; no laptop deploys.

### Epic 1.4 — Database schema & migrations
- [ ] Implement the full Phase-1 schema from `docs/specs/DATA_MODEL_DDL.md` as forward-only additive migrations (companies, company_verifications, company_financials, contacts, categories, leads, sourcing_requests, listings, buyer_profiles, documents, consents, activity_logs, users, templates). Include all indexes. Agent: `db-migration-engineer`; command `/new-migration`.
- [ ] Honor invariants: `(country, reg_no)` dedup; extensible enums (lookup tables); nullable verification/financials; `roles[]`; thin-lead → typed request.
- **DoD:** migrations apply clean on an empty DB; indexes present; invariants enforced by constraints.

### Epic 1.5 — DemoANAF adapter + B2B directory write path
> Provider audit + decision: `docs/specs/ANAF_API.md`. Use the DemoANAF **free** REST API (keyless, 300 req/min) — richer than the raw ANAF service the demo currently calls.
- [ ] **Repoint the adapter** from the raw ANAF web service (`webservicesp.anaf.ro`, VAT-only) to `https://demoanaf.ro/api/company/:cui`; map the rich response (ONRC reg, administrators, CAEN list, VAT status) into `companies` + `company_verifications`. Base URLs in env.
- [ ] Derive `companies.roles[]` from `caenCode` + `authorizedCaenCodes[]` via a **CAEN→roles/categories mapping table** (seeds the B2B directory + P2 matching).
- [ ] Optional `/company/:cui/financials` enrichment behind a config flag → `company_financials` (turnover, employees; used for lead scoring).
- [ ] Keep a **raw-ANAF fallback adapter** behind the same `CompanyDataProvider` port (DemoANAF has no published SLA).
- [ ] Cache + persist verification results; repeat CUIs hit cache, not the API.
- **DoD:** a real CUI verify persists a company + verification row with administrators + CAEN-derived roles; financials optional/nullable; provider swappable via the port.
- **Pre-handover:** confirm DemoANAF commercial-use terms / SLA; decide Free vs Pro/MCP; document the fallback. (See ANAF_API.md action items.)

### Epic 1.6 — Kick off Meta Business verification ⏳
- [ ] Start Meta Business verification (long-pole; runs in parallel through M4). Track status weekly.
- **DoD:** application submitted; owner + ETA recorded.

---

## M2 — Agent core + web widget `[P1]`

> Outcome: a user chats with the agent on the widget; messages dedupe; an LLM tool call executes; conversation state persists.

### Epic 2.1 — Channel-agnostic agent core
- [ ] Flow engine: per-vertical required-field definitions; computes missing fields each turn; drives the state machine (greeting→qualifying→verifying→confirmed/failed, +needs_human). Spec: `docs/specs/AI_AGENT_SPEC.md`.
- [ ] `LLM` port + **Claude adapter** (prod). Gemini adapter stays for demo. Model/temp/tokens/prompt-version from config. Agent: `agent-prompt-engineer`.
- [ ] Versioned prompt library (RO/EN per vertical), externalized from code (templates table or files), swappable without redeploy.
- [ ] Tools: `verify_company`, `upload_media`, `submit_lead`, `handoff_to_human` — args validated server-side before execution; LLM holds no keys.
- **DoD:** a scripted Angrosist conversation extracts fields and triggers a tool call against mock ports in tests.

### Epic 2.2 — Async runtime
- [ ] `Queue` adapter (Cloud Tasks). Handlers ack fast + enqueue; `worker` runs the turn.
- [ ] Idempotency: dedupe by provider message ID. Per-conversation lock (Postgres advisory lock; Redis optional later). Retryable vs terminal error classification.
- **DoD:** duplicate message IDs process once; concurrent events for one conversation serialize; worker retries transient failures.

### Epic 2.3 — Web widget + transport
- [ ] Embeddable `<script>` widget + standalone hosted page (reuse `frontend/widget/`). Real-time via WS or SSE + typing indicator. UI: shadcn per `docs/specs/UIUX_GUIDE.md`.
- [ ] Replace demo's session-storage-only flow with the documented widget-session contract; base URL via env.
- **DoD:** widget mounts on a third-party page, streams replies, survives reload.

---

## M3 — Angrosist + dashboard `[P1]` — **Angrosist LIVE**

> Outcome: Angrosist buyer works end-to-end on the widget; lead visible with transcript; offer status editable; handoff queues + notifies.

### Epic 3.1 — Angrosist buyer flow
- [ ] Full required-field flow (product, quantity, unit, delivery_location, recurring, deadline, budget, CUI, phone, email); in-conversation `verify_company`; `submit_lead` → `lead` + `contact` + `sourcing_request`.
- **DoD:** end-to-end qualify on the live widget creates a normalized lead.

### Epic 3.2 — Document upload
- [ ] `FileStore` (GCS) adapter + signed URLs; `upload_media` writes a `documents` row. Buyer product lists (Excel/Word) supported. Validate type/size. _Security._
- **DoD:** an uploaded file lands in GCS and is referenced by `documents.gcs_key`.

### Epic 3.3 — Consultant dashboard
- [ ] Lead pipeline (status/vertical/assigned/value/created) with server-side pagination + filters; lead detail (transcript + extracted fields + typed request + company verification panel); manual offer tracking (requested→sent→negotiation→won/lost + value + note); assign lead. Agent: `frontend-shadcn-builder`. Auth + RBAC (staff/admin).
- [ ] Companies / B2B directory view (roles[], verification, financials).
- **DoD:** staff log in, filter the pipeline, open a lead with transcript, edit offer status.

### Epic 3.4 — Email + handoff
- [ ] `Mailer` adapter (EU provider); confirmation email to prospect + internal notification to staff; RO/EN templates; SPF/DKIM/DMARC verified.
- [ ] Deferred handoff: `handoff_to_human` sets `needs_human=true`, `bot_active=false`; handoff queue in dashboard with full transcript; notify staff.
- **DoD:** lead submit sends both emails; handoff queues a lead and notifies staff.

---

## M4 — WhatsApp + PalletClearance + photos + i18n `[P1]`

> Outcome: both channels and both verticals live; seller flow blocks until photos; conversation works in RO and EN. Gated on Meta verification clearing.

### Epic 4.1 — WhatsApp channel
- [ ] `Channel` adapter for WhatsApp Cloud API: signature-verified webhook (`X-Hub-Signature-256`), verify-token challenge, fast ack + async processing, 24h-window handling, ≥1 re-engagement template. `wa.me` routing by vertical/intent. _Security._
- **DoD:** an inbound WhatsApp message qualifies through the same agent core; signatures enforced.

### Epic 4.2 — PalletClearance verticals
- [ ] Buyer flow (categories, volume, countries, near-expiry tolerance, subscribe→`buyer_profiles`). Seller flow (stock_type, category, quantity, location, expiry, target_price, confidential) → `listing`.
- [ ] **Mandatory seller photos block `submit_lead`** until ≥1 photo uploaded. Invariant.
- **DoD:** seller cannot complete without a photo; buyer subscribes to feed; listings appear in inventory.

### Epic 4.3 — i18n
- [ ] RO/EN detection + stickiness (stored on contact); dashboard + emails + agent localized.
- **DoD:** a full conversation completes in EN and in RO; language persists per contact.

---

## M5 — KPIs, security/GDPR, backup, testing, handover `[P1]`

> Outcome: KPIs compute; erasure cascades; restore drill succeeds; test suite green; docs delivered.

### Epic 5.1 — KPIs
- [ ] KPI dashboard: offers sent, conversion rate, pipeline value, tasks. Use indexed queries / read replica.
- **DoD:** KPIs compute correctly against seeded data.

### Epic 5.2 — GDPR & security
- [ ] Consent capture at first contact (`consents`); retention policy; **cascade erasure** (contact→leads→typed requests→documents in GCS→conversations/messages→redact `activity_logs`) per `docs/specs/SECURITY.md`. Command/agent: `security-gdpr-auditor`.
- [ ] Audit log of meaningful actions (full event catalog in SECURITY.md). RBAC enforced server-side. EU residency verified end-to-end.
- **DoD:** an erasure request removes all personal data + GCS files; audit entries written; RBAC denies cross-role access.

### Epic 5.3 — Reliability & tests
- [ ] Tested backup + restore drill (Cloud SQL PITR + GCS versioning). E2E tests on critical paths; basic load test at target concurrency (NFR-5).
- **DoD:** restore drill documented + passing; E2E + load green.

### Epic 5.4 — Documentation & handover
- [ ] Finalize README, CONTRIBUTING, API docs (openapi), runbooks, env inventory. Walkthrough with the client.
- **DoD:** a new developer can run + deploy from docs alone; client signs off.

### Phase-1 → Phase-2 gate
Phase 2 starts only when: P1 delivered & accepted; code in repo; docs + data model clear; dashboard + agent working; WhatsApp functional or status agreed; backup tested; open-bug list agreed.

---

## Phase 2 — SkalYou marketplace `[P2]`

> Reuses P1's agent, backend, DB additively. The P1 schema must already allow all of this (it does — keep it that way).

### M2.1 — Diagnostic + data model + basic provider portal
- [ ] Agent routes across 3 verticals; SkalYou diagnostic mode + `classify_need` tool (extract country/industry/urgency/value/specialist_type; escalate if unclassifiable).
- [ ] Additive tables: `market_entry_requests`, `providers`, `matches`, monetization **fields only**.
- [ ] Provider auth (Google + email OTP) + onboarding; **clickwrap before lead disclosure** (terms_version/IP/timestamp/user → `lead_terms_acceptance`); basic anti-circumvention logging.
- **DoD:** agent classifies a need + creates a `market_entry_request`; provider onboards; clickwrap recorded before client data is shown.

### M2.2 — Matching + dashboards + client magic-link
- [ ] Matching rule: best → accept-timeout → next → after 3 tries → escalate → manual allocation (Cloud Scheduler for timeouts/reminders).
- [ ] Provider dashboard (own leads, accept/decline, upload plan+offer). Internal staff view (all SkalYou leads, status, assigned provider, re-allocate, filter by country/market/status, decision history).
- [ ] Client **magic-link** (no account): view/accept/request-clarifications; expiring + access-logged JWT. _Security._
- [ ] Multi-country minimal (country fields, market tags, filter+match by country; RO active).
- **DoD:** lead routes to best provider, re-routes on refusal/timeout, escalates after 3 tries; provider uploads offer; client views + accepts via magic-link; staff filters by country + re-allocates.

### M2.3 — Testing, documentation, handover
- [ ] Verify all P2 acceptance criteria; tests green; docs delivered.

**P2 prep-only (schema allows, not built):** full monetization/billing, expert quality-score/availability/VIP logic, country-operator implementation, currency/VAT/VIES/legal localization.

---

## Phase 3 — First Country Operator `[P2+]`

Epic-level only: Country Operator role + scoped permissions; per-country dashboard view + filtering; per-country lead routing; basic localization for the launch market. Designed-for in Phase 2.

---

## Cross-cutting "definition of done" (every milestone)

- No hardcoded secrets/URLs (`/secret-scan` clean).
- All new I/O behind a port with a mock; the dependency rule holds.
- Migrations additive; indexes for new query paths.
- Input validation on new external inputs; parameterized SQL.
- New endpoints in `openapi.yaml`; new packages documented.
- Critical-path tests green; structured logs with correlation IDs, no PII.
- Security/GDPR review (`security-gdpr-auditor`) on anything touching personal data.
