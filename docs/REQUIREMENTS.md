# REQUIREMENTS.md — Requirements

Phase tags: **[P1]** Phase 1 MVP · **[P2]** Phase 2 SkalYou · **[P1-demo]** the initial demo module.

## Functional requirements

### FR-1 Channels & widget
- FR-1.1 [P1] Embeddable web chat widget (`<script>` snippet) mountable on any site. [P1-demo: hosted page form]
- FR-1.2 [P1] Standalone hosted chat page (same UI) for linking.
- FR-1.3 [P1] Real-time transport (WebSocket or SSE) with a typing indicator.
- FR-1.4 [P1] WhatsApp channel via Meta Cloud API: signature-verified webhook, fast ack + async processing, 24h-window handling, ≥1 re-engagement template.
- FR-1.5 [P1] `wa.me` click-to-chat links; routing by per-vertical number or by prefilled intent text.
- FR-1.6 [P1] Channel-agnostic agent core; both channels normalize into one conversation model.

### FR-2 Agent core & conversation
- FR-2.1 [P1] Hybrid flow: deterministic required-field definition per vertical + LLM for NLU, extraction, phrasing, language.
- FR-2.2 [P1] Tools the LLM may call: `verify_company`, `upload_media`, `submit_lead`, `handoff_to_human`. [P2] `classify_need`.
- FR-2.3 [P1] Async runtime: ingest → ack → enqueue → worker → LLM/tools → reply.
- FR-2.4 [P1] Idempotency by message ID; per-conversation lock; durable conversation state.
- FR-2.5 [P1] Guardrails: stay on task, no price promises, escalate on confusion / explicit request / verification failure.
- FR-2.6 [P1] RO/EN detection; conversation continues in one language; language stored on contact.

### FR-3 Vertical flows
- FR-3.1 [P1] **Angrosist buyer**: product, quantity, delivery location, recurring?, deadline, budget, CUI → verify → summarize → submit. [P1-demo]
- FR-3.2 [P1] **PalletClearance buyer**: categories, volume capacity, countries, near-expiry tolerance, subscribe-to-feed → submit.
- FR-3.3 [P1] **PalletClearance seller**: stock type, category, quantity, location, expiry, target price, confidentiality → **mandatory photos (blocks completion until provided)** → submit.
- FR-3.4 [P2] **SkalYou diagnostic**: open conversation → extract country, industry, urgency, estimated value, expert type → classify specialist type → mark incomplete / escalate if unclassifiable → save transcript.

### FR-4 Company verification & B2B database
- FR-4.1 [P1] Verify by CUI via DemoANAF `GET /api/company/:cui` (name, VAT status, administrators, CAEN). [P1-demo]
- FR-4.2 [P1] Optional financials/turnover via `GET /api/company/:cui/financials`.
- FR-4.3 [P1] Cache verification results; persist into `companies` / `company_verifications` / `company_financials`.
- FR-4.4 [P1] Maintain the B2B company directory; tag `companies.roles[]` opportunistically as companies flow through verticals.

### FR-5 Lead capture & documents
- FR-5.1 [P1] Each qualified conversation → normalized `lead` + `company` + `contact` + typed request (`sourcing_request` | `listing`).
- FR-5.2 [P1] Upload buyer product lists (Excel/Word) and seller photos to GCS; store references.
- FR-5.3 [P1] Capture consent; record `activity_logs`.

### FR-6 Human handoff
- FR-6.1 [P1] Escalation sets `needs_human`, sets `bot_active=false` (mutes bot for that conversation), notifies staff.
- FR-6.2 [P1] Handoff queue in dashboard with full transcript; staff follow up (deferred model).
- FR-6.3 [P1] (Hook only) `bot_active` enables later live takeover without rework.

### FR-7 Dashboard (staff/consultant)
- FR-7.1 [P1] Lead pipeline with statuses; lead detail with transcript + extracted fields. [P1-demo: minimal list]
- FR-7.2 [P1] Companies (B2B DB) view; listings inventory.
- FR-7.3 [P1] **Manual offer tracking**: status (requested → sent → negotiation → won/lost) + value + note.
- FR-7.4 [P1] KPIs (offers sent, conversion rate, pipeline value), tasks, roles.
- FR-7.5 [P1] Handoff queue.

### FR-8 Email & deliverability
- FR-8.1 [P1] Transactional email: lead confirmation to prospect + internal notification to staff.
- FR-8.2 [P1] Deliverability: SPF, DKIM, DMARC, return-path, dedicated sending subdomain; RO/EN templates.

### FR-9 SkalYou marketplace [P2]
- FR-9.1 Matching engine: best provider by role + category + market; rule = best → accept-timeout → next → after 3 tries → escalate to staff → manual allocation.
- FR-9.2 Provider portal: auth via **Google + email OTP**; onboarding (profile, categories/roles/markets, consent to receive leads); provider dashboard (own leads only, accept/decline, upload plan+offer).
- FR-9.3 **Clickwrap** before lead disclosure: accept commercial terms (checkbox, timestamp, IP, user ID, terms version, logged) before seeing full client data.
- FR-9.4 Client **magic-link** (no account): view offer, **accept**, **request clarifications** (message + `clarificare solicitată` status + notification); link expires; access logged.
- FR-9.5 Extended state machine: new → diagnosed → matched → routed → accepted/declined → offer-uploaded → delivered → client-accepted. Cron for timeouts/reminders.
- FR-9.6 Multi-country **minimal**: country fields (client, expert, target market), language, market tags, dashboard filter by country, matching by country/market; RO active at launch.
- FR-9.7 Monetization **fields only**: provider plan (Starter/PRO/VIP), commission, offer value, estimated commission, transaction status, lead-accepted timestamp, terms acceptance. No billing logic.
- FR-9.8 Anti-circumvention logging: unique lead ID, lead-acceptance log, client-data-view log, offer-upload log, status.

## Non-functional requirements

- NFR-1 **i18n**: RO/EN across conversation, email, dashboard; data model carries country/market for extension.
- NFR-2 **Security**: signature-verified webhooks; role-based access; secrets only in Secret Manager; least-privilege IAM; TLS everywhere; expiring access-logged magic-links.
- NFR-3 **GDPR**: explicit consent; company-vs-personal data separation; retention; cascade erasure (contact→lead→conversation→files); audit log; **EU data residency** (GCP `europe-*`, EU email region).
- NFR-4 **Reliability**: idempotent message handling; retry/backoff on external calls; durable queue; tested backup + restore.
- NFR-5 **Performance/scale**: target load is modest (single-digit to low-double-digit concurrent chats; the real ceiling is the LLM, not the instance). A single backend instance is acceptable; the design must stay horizontally scalable (stateless services, externalized state) so scaling later is a deploy change, not a rewrite.
- NFR-6 **Availability**: single-instance acceptable for MVP; managed Cloud SQL backups + PITR; server-down handled per the maintenance SLA (critical).
- NFR-7 **Observability**: structured logs (Cloud Logging), metrics (Cloud Monitoring), errors (Sentry), correlation IDs.
- NFR-8 **Portability / clean architecture**: hexagonal; all vendors behind ports; 12-factor.

## Out of scope (explicit)

Website builds/redesign/SEO; quote generation/sending/e-sign; payments & commission billing; voice messages; automated nurture sequences; advanced analytics beyond core KPIs; full negotiation/counter-offer; commercial multi-country (country operators, currency, VAT/VIES, legal localization); advanced matching (scoring, availability, parallel fan-out, VIP logic); expert tier packages; full audit export; ongoing maintenance after handover (separate SLA).
