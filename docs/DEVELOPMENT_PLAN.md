# DEVELOPMENT_PLAN.md — Phased Plan & Milestones

Build inside-out per milestone (domain → adapters → API → UI). Each milestone lists **deliverables** and **acceptance criteria (AC)**. A milestone is "done" only when its AC pass and the critical paths are tested.

---

## Commercial terms (agreed with client)

- **Payments on delivery, not in advance** — each milestone is paid when delivered and accepted (no upfront avans). Keep milestones small/frequent and define "delivery/acceptance" clearly so payment is unambiguous.
- **Faza 1** (Angrosist + PalletClearance): **€5.000**, on milestones.
- **Faza 2** (SkalYou Core MVP): **€5.000**, on milestones.
- **Faza 3** (first Country Operator launch): **€2.000**, on milestones.
- **Maintenance:** **€280/lună**, 7h included, €40/h extra, + monthly roadmap meeting and involvement in technical decisions.
- **Performance bonuses:** €500 (first enterprise client), €500 (agreed monthly-revenue threshold), annual bonus on agreed KPIs.
- **No equity at this stage** (open to discuss later).
- **Trial first:** a 1–2 week small fixed-scope module before the larger contract — this is **Milestone 0** below.

---

## Milestone 0 — Demo module / collaboration trial (skills showcase, ~1–2 weeks)

A thin end-to-end vertical slice on the **real (minimal) data model** — it becomes Phase 1's first slice, not throwaway.

**Deliverables**
- Hosted web chat page (single vertical: Angrosist buyer, RO).
- Agent qualifies via natural conversation (extracts product, quantity, location, CUI from free text).
- Live company verification via DemoANAF `GET /api/company/:cui` (company endpoint only for the demo).
- Backend writes to real (thin) schema: `companies`, `contacts`, `leads`, `sourcing_requests`.
- Minimal dashboard: list of leads + transcript + extracted fields.

**Deploy (demo only):** may use Vercel + Neon for speed, OR go straight to GCP. (Production is GCP — see below.)

**AC**
- A clickable link; the agent qualifies an Angrosist buyer end-to-end; one real CUI verification with correct data; the lead appears in the dashboard with transcript + fields.

---

## Phase 1 — MVP (both verticals, both channels) — €5.000

**Long-pole dependency:** start **Meta Business verification** at M1 and run it in parallel — it gates the WhatsApp channel (M4).

### M1 — Foundation & infra
- Deliverables: GCP projects (dev/staging/prod) via Terraform; Cloud SQL; Artifact Registry; Cloud Run service skeleton (Docker image); Secret Manager; CI/CD (build → registry → deploy); DB schema + migrations; DemoANAF verification adapter; B2B company directory write path; Cloudflare + DNS + email auth records (SPF/DKIM/DMARC); kick off Meta verification.
- AC: backend deploys to staging from CI; migrations run; a CUI verify call persists a company; secrets resolve from Secret Manager.

### M2 — Agent core + web widget
- Deliverables: channel-agnostic agent (flow engine + LLM port + tools); async runtime (Cloud Tasks worker, idempotency, per-conversation lock); web widget (embeddable script + standalone page) with WS/SSE.
- AC: a user can chat with the agent on the widget; messages dedupe; an LLM tool call executes; conversation state persists.

### M3 — Angrosist + dashboard
- Deliverables: Angrosist buyer flow; in-conversation verification; document upload to GCS; lead → backend → dashboard (pipeline, lead detail + transcript, manual offer tracking); confirmation + internal email; deferred handoff (`needs_human`, `bot_active`, queue).
- AC: **Angrosist LIVE on the widget** end-to-end; lead visible with transcript; offer status editable; handoff queues a lead and notifies staff.

### M4 — WhatsApp + PalletClearance + photos + multilingual
- Deliverables: WhatsApp channel (Cloud API, webhook + signature, templates) once Meta verification clears; PalletClearance buyer + seller flows; **mandatory seller photos**; RO/EN throughout.
- AC: both channels and both verticals LIVE; seller flow blocks until photos uploaded; conversation works in RO and EN.

### M5 — KPIs, security/GDPR, backup, testing, handover
- Deliverables: KPI dashboard; GDPR (consent, retention, cascade erasure, audit, roles); backup + **tested restore**; end-to-end + load testing; documentation; handover.
- AC: KPIs compute; erasure cascades; a restore drill succeeds; test suite green; docs delivered.

**Payment (P1):** €5.000 paid on delivery, no advance — split across milestones (suggested payment points: M1 foundation, Angrosist LIVE at M3, handover at M5).

---

## Maintenance / SLA (operational, post-handover) — €280/lună, ≥12 months

Includes **7h/month** (bug fixing, minor adjustments, prompt + security updates, external-API change adaptation, server-down = critical, monitoring, monthly reporting, backup/restore checks). Overtime **€40/h**. Plus a **monthly roadmap meeting and involvement in technical decisions**. New functionality quoted separately. Full terms in the signed SLA. Not a build milestone — listed for completeness.

**Performance bonuses:** €500 (first enterprise client), €500 (agreed monthly-revenue threshold), annual bonus on agreed KPIs.

---

## Phase-1 → Phase-2 Gate (must pass before SkalYou)

Phase 2 starts only when: Phase 1 delivered & accepted; code in repo; docs + data model clear; dashboard + agent (widget) working; WhatsApp functional or status agreed; backup tested; open-bug list agreed.

---

## Phase 2 — SkalYou Core MVP — €5.000

Reuses Phase 1's agent, backend, and database (additive). Working for Romania, ready for extension.

### M2.1 — Diagnostic + data model + basic provider portal
- Deliverables: agent routing across the 3 verticals; SkalYou diagnostic mode (classify specialist type, extract country/industry/urgency/value, escalate if unclassifiable); `market_entry_requests`, `providers`, `matches`, monetization **fields**; provider auth (Google + email OTP) + onboarding; **clickwrap** before lead disclosure; basic anti-circumvention logging.
- AC: agent classifies a need and creates a `market_entry_request`; a provider signs up and onboards; clickwrap is recorded (timestamp/IP/version) before client data is shown.

### M2.2 — Matching + dashboard + client magic-link
- Deliverables: matching rule (best → timeout → next → 3 tries → escalate → manual); provider dashboard (own leads, accept/decline, upload plan+offer); internal staff dashboard (all SkalYou leads, status, assigned provider, re-allocate, offers, filter by country/market/status, decision history); client **magic-link** (view + accept + request-clarifications, expiring + logged); multi-country **minimal** (country fields, market tags, filter + matching by country; RO active); cron for timeouts/reminders.
- AC: a lead routes to the best provider, re-routes on refusal/timeout, escalates after 3 tries; provider uploads an offer; client views and accepts/requests-clarifications via magic-link; staff can filter by country and re-allocate.

### M2.3 — Testing, documentation, handover
- AC: acceptance criteria above verified; tests green; docs delivered.

**Payment (P2):** €5.000 paid on delivery, no advance — split across milestones (M2.1 diagnostic + data model + basic provider portal · M2.2 matching + dashboard + client magic-link · M2.3 testing + handover).

**Phase-2 prep-only (schema allows, not built):** full monetization/billing, expert quality-score/availability/VIP logic, country-operator implementation + per-country isolation/reporting + currency/VAT/VIES/legal localization.

**Phase-2 follow-ups (separate quotes):** full negotiation, billing system, commercial multi-country, advanced matching, expert tier packages, full audit export.

---

## Phase 3 — First Country Operator launch — €2.000

Activates the Country Operator model for the first additional market (designed-for in Phase 2). Detailed scope + acceptance criteria are defined at kickoff; indicatively:
- Country Operator role + scoped permissions (an operator sees/manages only their country).
- Per-country dashboard view + filtering; per-country lead routing.
- Basic localization for the launch market (language + market config).
- Onboarding the first country operator and their local providers.

Paid on delivery, on milestones.

---

## Sequencing notes

- Build the **web widget before WhatsApp** — it has no external approval dependency, so Angrosist can go live (M3) while Meta verification is still pending.
- Keep the data model **additive**: M-by-M changes must not reshape existing company/lead data.
- The B2B directory + `roles[]` tagging must ship in Phase 1 — Phase 2's €5.000 and SkalYou matching depend on it being populated.
