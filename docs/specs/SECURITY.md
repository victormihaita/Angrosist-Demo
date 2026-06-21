# SECURITY.md — Security & Compliance Specification

> Scope: Euro Intermed B2B platform (Angrosist, PalletClearance — P1; SkalYou — P2).
> Companion specs: `REQUIREMENTS.md` (NFR-2 Security, NFR-3 GDPR), `ARCHITECTURE.md`, `DATA_MODEL.md`, root `CLAUDE.md` §6–§7 + §9 invariants.
> This document is **checklist-driven** — it doubles as the audit checklist. Every `- [ ]` is an auditable control.
> Phase tags: **[P1]** Phase 1 MVP · **[P2]** Phase 2 SkalYou · **[M0]** demo-only.

---

## 0. Security baselines (non-negotiable)

These are the project invariants from `CLAUDE.md` §7 and `REQUIREMENTS.md` NFR-2/NFR-3, restated as hard rules:

- [ ] **No secret is ever hardcoded, committed to git, or written to logs.** Secret Manager in prod; gitignored `.env` in demo only.
- [ ] **Webhooks are signature-verified on every request** (WhatsApp `X-Hub-Signature-256`).
- [ ] **The LLM never holds a credential or a DB handle.** It requests actions through tools; our code executes them. (`ARCHITECTURE.md` §1.)
- [ ] **Role-based access** on the dashboard API and provider portal.
- [ ] **TLS everywhere**, least-privilege IAM per Cloud Run service account, CORS locked to known origins.
- [ ] **EU data residency** — GCP `europe-*` regions, EU email region.
- [ ] **GDPR**: explicit consent, company/personal data separation, retention, cascade erasure, audit log.
- [ ] **Magic-links (P2)** expire and are access-logged; single-use.

---

## 1. Threat model (STRIDE-lite)

STRIDE = **S**poofing, **T**ampering, **R**epudiation, **I**nfo disclosure, **D**enial of service, **E**levation of privilege.
One row per attack surface. Each lists the realistic threats and the mandated control(s).

### 1.1 Public chat endpoint (`POST /api/chat`, widget WS/SSE) [P1]
| STRIDE | Threat | Control |
|---|---|---|
| S | Forged session / impersonating another visitor's conversation | Server-issued opaque conversation/session token; never trust client-supplied `conversation_id` without server-side ownership check |
| T | Tampering with message payload, oversized body | Strict input validation (§6); body size cap; reject malformed JSON |
| R | Visitor denies submitting a lead | `activity_logs` entry on lead creation with IP + timestamp; consent record |
| I | Leaking other leads/companies via the chat response | Agent core only ever sees the current conversation's state; no cross-conversation reads |
| D | Flood of messages / LLM cost-exhaustion (each turn = paid LLM call) | Per-IP + per-conversation rate limit; Cloudflare WAF; max turns per conversation; message size cap |
| E | Driving the agent to call tools beyond its scope | Tool allow-list per vertical; tool-arg validation (§6.4); LLM holds no keys |

- [ ] Conversation ownership enforced server-side (no client-asserted identity)
- [ ] Request body size cap (e.g. 32 KB chat message)
- [ ] Rate limiting per IP and per conversation
- [ ] Max turns / max conversation lifetime to bound LLM spend

### 1.2 WhatsApp webhook (`POST /api/webhooks/whatsapp`, `GET` verify) [P1]
| STRIDE | Threat | Control |
|---|---|---|
| S | Forged inbound webhook (attacker posts fake Meta payloads) | **HMAC-SHA256 `X-Hub-Signature-256` verification on every POST** (§4); reject 403 |
| S | Forged subscription handshake | Verify-token challenge on `GET` (§4); 403 on mismatch |
| T | Replayed / duplicated messages | Idempotency by provider message ID (`FR-2.4`); dedupe store |
| R | Dispute over message receipt | `activity_logs` + durable conversation/message store |
| I | App secret / token disclosure | Secrets in Secret Manager only (§2); never logged |
| D | Webhook flood | Fast ack (HTTP 200) + async enqueue (Cloud Tasks); Cloudflare in front |
| E | — | Webhook handler does no privileged action directly; only enqueues |

- [ ] Signature verified before any processing or logging of body content
- [ ] GET verify-token check returns the `hub.challenge` only on exact token match
- [ ] Fast 200 ack; all work enqueued (no LLM call in the webhook handler)
- [ ] Message-ID dedupe before enqueue

### 1.3 Dashboard API (`/api/leads`, companies, handoff, offer status) [P1]
| STRIDE | Threat | Control |
|---|---|---|
| S | Unauthenticated access (**CURRENT M0 GAP — no auth**) | Require authenticated staff/admin session/JWT on every dashboard route (§3) |
| T | Modifying another tenant's / unassigned lead | RBAC checks (§3); `assigned_to` scoping for staff |
| R | Staff denies a status change / handoff | `activity_logs` with actor_id on every mutation (§8) |
| I | Bulk lead/PII export by low-privilege user | RBAC; staff see assigned-only where configured; no PII in URLs/logs |
| D | — | Cloudflare WAF; authn gate sheds anonymous load |
| E | Staff escalating to admin actions (manage users) | Capability checks per route (§3 matrix), not just "is logged in" |

- [ ] **FIX M0 GAP:** no dashboard endpoint is reachable without authentication
- [ ] Every mutating route checks the actor's capability against the RBAC matrix (§3)
- [ ] Every mutation writes an `activity_logs` row (§8)

### 1.4 Provider / expert portal [P2]
| STRIDE | Threat | Control |
|---|---|---|
| S | Account takeover | Auth via Google + email OTP (`FR-9.2`); short-lived sessions |
| T | Provider accepting a match not routed to them | Server validates the match belongs to the provider before accept/decline |
| R | Provider denies viewing client data (anti-circumvention) | Clickwrap `lead_terms_acceptance` + client-data-view log (§8) |
| I | **Seeing client PII before clickwrap** | **Hard gate: full client data withheld until `lead_terms_acceptance` recorded** (invariant) |
| D | — | Rate limits; WAF |
| E | Provider viewing other providers' leads | Strict "own leads only" scoping (`FR-9.2`) |

- [ ] Provider sees own leads only
- [ ] Full client data disclosed only after clickwrap row exists (`lead_terms_acceptance`)
- [ ] Match accept/decline validates provider ownership of the match

### 1.5 Client magic-link page [P2]
| STRIDE | Threat | Control |
|---|---|---|
| S | Guessing / forging a link | Signed JWT (HMAC), high-entropy, audience-scoped (§5) |
| T | Altering `lead_id` in the token | HMAC signature covers all claims; reject on signature mismatch |
| R | Client denies accepting an offer | Access + action logged (`FR-9.8`, §8) |
| I | Link leaks via referrer/history → data exposure | Short expiry; **single-use**; `410 Gone` after use/expiry; no PII in URL beyond opaque token |
| D | Link-brute-force | Signature makes guessing infeasible; rate-limit the endpoint |
| E | — | Token grants view/accept on one lead only; no portal access |

- [ ] Token is a signed, expiring, audience-scoped JWT (§5)
- [ ] Single-use semantics enforced server-side
- [ ] Expired/used link returns `410 Gone`; every access logged

### 1.6 File uploads (buyer product lists, seller photos, P2 offers) [P1]
| STRIDE | Threat | Control |
|---|---|---|
| S | Uploading on behalf of another lead | Upload bound to the authenticated/owning conversation or session |
| T | Malicious file (polyglot, oversized) | MIME allow-list + magic-byte sniff; size cap; reject executables |
| R | — | `activity_logs` file-upload event (§8) |
| I | Public bucket / guessable object URLs | Private GCS bucket; **signed URLs** only (`ARCHITECTURE.md`); EU region |
| D | Storage-exhaustion upload flood | Per-conversation file count + size caps |
| E | Path traversal in stored key | Server generates `gcs_key`; never trust client filename |

- [ ] MIME allow-list + content sniff (images for photos; xlsx/docx/pdf for product lists)
- [ ] Per-file and per-conversation size limits
- [ ] Files in private GCS, served via signed URLs only, never public
- [ ] Server-generated object keys (no client-controlled paths)
- [ ] **Seller-photo gate:** PalletClearance seller flow cannot complete without photos (`FR-3.3`, invariant)

### 1.7 The LLM (prompt injection) [P1]
| STRIDE | Threat | Control |
|---|---|---|
| S | User pretends to be "system"/"admin" in chat | System-prompt hardening; user content is always untrusted data, never instructions |
| T | Injected text steering the agent off-task or to exfiltrate | Guardrails (`FR-2.5`): stay on task, no price promises, escalate on confusion |
| R | — | Conversation + tool-call logging |
| I | Tricking the agent into revealing other leads / system prompt / secrets | LLM has no access to other conversations, no secrets, no DB handle (`ARCHITECTURE.md` §1) |
| D | Prompt that forces many expensive tool loops | Max tool-calls per turn; max turns per conversation |
| E | Injection that triggers `submit_lead`/`verify_company` with attacker args | **Tool-arg validation server-side** (§6.4); tools re-validate every argument; CUI re-checked against ANAF |

- [ ] System prompt explicitly states user messages are data, not instructions
- [ ] Tool allow-list per vertical; unknown tool calls rejected
- [ ] Every tool argument validated server-side before execution (§6.4)
- [ ] LLM cannot read secrets, env, other conversations, or hold a DB/API handle
- [ ] Bounded tool-call and turn counts per conversation

---

## 2. Secret inventory & management

**Rule (absolute):** every secret lives in **Secret Manager (prod/staging)** or a **gitignored `.env` (demo/local only)**. NEVER hardcoded, NEVER committed to git, NEVER logged, NEVER in client-side code or env baked into the frontend bundle.

### 2.1 Inventory
| Secret | Used by | Store (prod) | Notes |
|---|---|---|---|
| `DATABASE_URL` (Cloud SQL / Neon creds) | backend | Secret Manager | Includes DB password; use Cloud SQL IAM auth where possible |
| LLM API key (`GEMINI_API_KEY` [M0] → Anthropic `ANTHROPIC_API_KEY` [P1+]) | backend agent core | Secret Manager | M0 uses Gemini; P1+ uses Claude |
| WhatsApp **access token** | whatsapp adapter | Secret Manager | Long-lived / system-user token |
| WhatsApp **app secret** | webhook signature verify (§4) | Secret Manager | HMAC key for `X-Hub-Signature-256` |
| WhatsApp **verify token** | webhook GET challenge (§4) | Secret Manager | Operator-chosen string |
| Email provider API key (SendGrid/Mailgun/Brevo) | mailer adapter | Secret Manager | EU region account |
| **JWT signing secret** (magic-link + sessions) [P2] | api / magic-link (§5) | Secret Manager | HMAC key; rotate independently |
| GCS access / service-account key | storage adapter | Workload Identity (preferred) / Secret Manager | Prefer attached SA + Workload Identity over key files |
| Cloud Run service-account credentials | runtime | GCP-managed (attached SA) | No exported key files |
| Sentry DSN | observability | Secret Manager / env | Low-sensitivity but treat as config secret |

### 2.2 Management rules
- [ ] All of the above are referenced by name from Secret Manager at runtime (prod/staging)
- [ ] Local/demo secrets only in `backend/.env` / `frontend/.env`, both gitignored
- [ ] No secret printed in logs, error messages, Sentry breadcrumbs, or stack traces (scrub before send)
- [ ] No secret in the frontend bundle (only `VITE_API_URL`-style non-secret config is allowed client-side)
- [ ] Each environment (dev/staging/prod) has its own GCP project and its own secret values (`CLAUDE.md` §6)
- [ ] Secret access via least-privilege IAM (only the owning service account can read a given secret)

### 2.3 Rotation procedure
1. Generate the new secret value at the provider (DB, Anthropic/Gemini, Meta, email, etc.).
2. Add a **new version** in Secret Manager (do not delete the old version yet).
3. Deploy/restart the consuming Cloud Run service so it picks up the latest version.
4. Verify health (auth succeeds, webhooks verify, LLM calls succeed).
5. **Disable** then later **destroy** the old Secret Manager version.
6. Record the rotation in `activity_logs` (system actor) and the run-book.
- [ ] Rotation cadence: at least every 90 days, **and immediately on any suspected exposure**.
- [ ] JWT signing-secret rotation invalidates outstanding magic-links — acceptable (they are short-lived; document it).

### 2.4 ⚠️ CURRENT LEAK — remediation backlog (M0)
**Finding:** `backend/.env` exists in the working tree and contains a **real `GEMINI_API_KEY` value** (and is intended to hold Neon DB creds). The brief notes a real Gemini key + Neon password are present in the working-tree `.env`.

**Current mitigating state (verified):**
- `backend/.env` and `frontend/.env` are listed in root `.gitignore` ("env files — never commit real values").
- `git ls-files` shows `backend/.env` is **not tracked** by git; the committed `backend/.env.example` is clean (empty `GEMINI_API_KEY=`, placeholder `DATABASE_URL`).

**Remediation (do in order):**
1. [ ] **ROTATE the Gemini API key now** (assume exposed — a real key sitting in a working-tree file is a leak risk even if untracked). Generate a new key, store in Secret Manager / local `.env`, revoke the old key at the provider.
2. [ ] **ROTATE the Neon DB password** if any real credential was ever placed in `backend/.env`; update Secret Manager / `.env`.
3. [ ] **Confirm gitignore coverage** — verify `git check-ignore backend/.env frontend/.env` returns the paths; verify `git log --all -- backend/.env` shows no historical commit of the file. If it was ever committed, purge from history (`git filter-repo`) and force-push, then rotate again.
4. [ ] **Scrub for accidental commits** of secrets repo-wide (run secret scanning — §10).
5. [ ] Keep only `backend/.env.example` (placeholders) in git going forward.

---

## 3. RBAC matrix

Roles (P1): `staff`, `admin`. Roles added in P2 (data model already allows — `DATA_MODEL.md`): `provider`/`expert`, `country_operator`, `admin_global`.
Legend: ✅ allow · ❌ deny · ➖ N/A · *scoped* = allowed but limited to own/assigned/country records.

| Capability | staff [P1] | admin [P1] | provider/expert [P2] | country_operator [P2] | admin_global [P2] |
|---|---|---|---|---|---|
| View leads (all) | ✅ | ✅ | ❌ | *country-scoped* | ✅ |
| View **assigned-only** leads | ✅ (own) | ✅ | ✅ (own routed) | *country* | ✅ |
| View company / B2B directory | ✅ | ✅ | *own categories/markets* | *country* | ✅ |
| Edit offer / offer status (manual tracking) | ✅ | ✅ | ➖ | *country* | ✅ |
| Handoff (claim/work handoff queue) | ✅ | ✅ | ❌ | *country* | ✅ |
| Manage users / roles | ❌ | ✅ | ❌ | ❌ | ✅ |
| Accept / decline a routed match | ➖ | ➖ | ✅ (own match) | ❌ | ✅ |
| Upload offer (plan + offer doc) | ➖ | ✅ | ✅ (own lead) | *country* | ✅ |
| View client data **gated by clickwrap** | ✅ | ✅ | ✅ **only after `lead_terms_acceptance`** | *country, post-clickwrap* | ✅ |
| Country-scoped access enforcement | ➖ (global P1) | ➖ | ➖ | ✅ (own country only) | ✅ (all countries) |
| Trigger data-subject erasure | ❌ | ✅ | ❌ | ❌ | ✅ |
| View audit / activity logs | *own actions* | ✅ | ❌ | *country* | ✅ |

- [ ] Authorization checked server-side per request (never trust client role claims)
- [ ] `staff` is assignment-scoped where configured; `admin` is global within P1
- [ ] `provider` is hard-scoped to own routed leads/matches and gated by clickwrap
- [ ] `country_operator` cannot read outside its country; `admin_global` spans countries
- [ ] User-management and erasure are admin/`admin_global` only

---

## 4. Webhook signature verification (WhatsApp) [P1]

WhatsApp Cloud API signs each POST with `X-Hub-Signature-256: sha256=<hex HMAC>`, keyed by the **app secret**, over the **raw request body**.

**POST (inbound messages):**
- [ ] Read the **raw** body bytes (compute HMAC before JSON parsing; parsing must not alter bytes).
- [ ] Compute `HMAC-SHA256(app_secret, raw_body)`; compare to the `sha256=` value using **constant-time** comparison.
- [ ] On mismatch or missing header: **reject `403`**, log the rejection (no body content, just metadata), do **not** enqueue.
- [ ] On success: ack **`200`** fast, dedupe by message ID, enqueue to Cloud Tasks (no LLM in handler).

**GET (subscription verification handshake):**
- [ ] Compare `hub.verify_token` to the configured verify token (constant-time).
- [ ] On match: return `hub.challenge` verbatim with `200`.
- [ ] On mismatch: return `403`, log the attempt.

- [ ] App secret and verify token come from Secret Manager (§2), never inline.
- [ ] Signature verification runs on **every** webhook request, with no bypass flag in prod.

---

## 5. Magic-link design [P2]

Purpose: let a client (no account) view an offer, accept it, or request clarifications (`FR-9.4`), via a single signed link.

**Token = JWT, HMAC-signed (HS256) with the JWT signing secret (§2).**

Claims:
| Claim | Meaning |
|---|---|
| `sub` / `lead_id` | the single lead/offer this link grants access to |
| `aud` | audience = `client-magic-link` (reject tokens minted for other audiences) |
| `exp` | expiry (short — e.g. 7 days; tune per business) |
| `iat` | issued-at |
| `jti` | unique token ID — basis for single-use tracking |

Rules:
- [ ] Signed with HMAC (HS256); signature covers all claims — any tampering (e.g. changing `lead_id`) fails verification.
- [ ] Server verifies signature, `aud`, and `exp` on every request.
- [ ] **Single-use semantics:** track `jti` (or a server-side link record) and mark consumed after first successful use; reject reuse.
- [ ] **Expired or already-used link → `410 Gone`.**
- [ ] **Every access and action (view / accept / clarification) logged** to `activity_logs` (`FR-9.4`, `FR-9.8`, §8) with lead_id, IP, timestamp.
- [ ] Token grants access to exactly one lead's offer view/accept/clarify — nothing else; no portal/session escalation.
- [ ] No PII in the URL beyond the opaque token.

---

## 6. Input validation & injection defense

**Rule:** validate and sanitize **all** external input — chat messages, CUI, file metadata, emails, phone numbers, webhook payloads, dashboard form fields, magic-link tokens. Reject (don't silently coerce) malformed input.

### 6.1 Field-level validation
- [ ] **Chat message:** UTF-8, length cap, strip/normalize control chars; treated as untrusted data (never as agent instructions).
- [ ] **CUI:** Romanian CUI format (numeric, length/checksum), then **server-side re-verify against DemoANAF** — never trust an LLM-extracted CUI without an independent verify call.
- [ ] **Email:** RFC-valid format; normalize; length cap.
- [ ] **Phone:** E.164 normalization; reject non-conforming.
- [ ] **File:** type against MIME allow-list **+ magic-byte sniff**; size cap; reject executables/scripts (§1.6).
- [ ] **Country / enum fields:** validated against the allowed enum set (extensible enums per `DATA_MODEL.md`, but still validated).
- [ ] **Quantities / numerics / dates:** typed parsing with range checks.

### 6.2 SQL injection
- [ ] **Parameterized queries only** — no string concatenation into SQL anywhere.
- [ ] Repositories use bound parameters; dynamic identifiers (table/column) never come from user input.

### 6.3 XSS (frontend)
- [ ] Render all user/lead-derived content as text (React default escaping); **no `dangerouslySetInnerHTML`** with untrusted data.
- [ ] Sanitize any rich content (e.g. transcript) before display.
- [ ] Set a Content-Security-Policy (§9) to constrain script sources.

### 6.4 LLM prompt-injection defense
- [ ] **System-prompt hardening:** instructions state user content is data; agent stays on task; never reveals system prompt, secrets, or other conversations (`FR-2.5`).
- [ ] **Tool allow-list** per vertical (`verify_company`, `upload_media`, `submit_lead`, `handoff_to_human`; P2 `classify_need`) — reject any other tool name.
- [ ] **Every tool argument re-validated server-side** before execution (CUI re-verified, IDs ownership-checked, sizes/types checked) — the LLM's output is never trusted blindly.
- [ ] **The LLM never holds a key or a DB handle** — all access is mediated by our tool executors (`ARCHITECTURE.md` §1, `CLAUDE.md` §9).
- [ ] Bounded tool-calls/turn and turns/conversation to cap cost-exhaustion injections.

---

## 7. GDPR compliance

### 7.1 Consent
- [ ] **Explicit consent captured at first contact**, before processing personal data (`CLAUDE.md` §7).
- [ ] Recorded in `consents` (`contact_id`, `text_version`, `given_at`, `channel`, `ip`) — version + timestamp + IP.
- [ ] Consent event also written to `activity_logs` (§8).

### 7.2 Company (public) vs personal (personal) data separation
- [ ] **Public/company data** — `companies`, `company_verifications`, `company_financials` (CUI, name, VAT, CAEN, administrators, turnover): business data, lawful basis = legitimate interest / public registry.
- [ ] **Personal data** — `contacts` (name, phone, email, language), `consents` (IP), conversation/message content, `lead_terms_acceptance` (IP, user_id): tied to a data subject; erasable.
- [ ] Erasure targets personal data; company directory entries may be retained (anonymized of personal links).

### 7.3 Retention policy
- [ ] Define and document a retention period per data class (e.g. raw conversations/messages: shorter; normalized leads: business-defined; company directory: retained as the strategic asset).
- [ ] Scheduled job (Cloud Scheduler) purges data past retention.
- [ ] Retention periods documented and surfaced in the privacy notice.

### 7.4 Cascade-erasure procedure (right to erasure)
Triggered by an admin/`admin_global` on a data-subject request. Forward-only, additive model (`DATA_MODEL.md` invariant) — erasure deletes/anonymizes; it does not reshape schema.

Step by step (in order, transactional where feasible):
1. [ ] Resolve the **contact** from the request (the data subject).
2. [ ] Find all **leads** for that contact.
3. [ ] For each lead, delete/anonymize its **typed request** (`sourcing_requests` | `listings` | P2 `market_entry_requests`).
4. [ ] Delete associated **documents in GCS** by `documents.gcs_key` (object delete; account for object versioning — purge versions), then the `documents` rows.
5. [ ] Delete the lead's **conversations and messages** (transcript content is personal data).
6. [ ] P2: delete/anonymize **offers**, **matches**, and `lead_terms_acceptance` tied to the lead where personal.
7. [ ] Delete the **contact** record (and `consents` for that contact, or retain a minimal consent-revocation proof if required).
8. [ ] **Redact `activity_logs`** — retain the audit entries (legal/anti-circumvention need) but **redact PII** from `meta`, replacing direct identifiers with the erased subject's pseudonymous ID. Do not delete the audit trail wholesale.
9. [ ] Optionally retain the `companies` directory row (public data) but sever personal links.
10. [ ] Write a final `activity_logs` erasure event (system/admin actor): `action=data_erasure`, subject pseudonymous ID, timestamp — proof the request was honored.

- [ ] Erasure is admin-only (§3) and itself audited.
- [ ] GCS object versioning is accounted for so "deleted" files are truly gone.

### 7.5 EU data residency checklist
- [ ] Cloud SQL (Postgres) in a GCP **`europe-*`** region (EU data residency, `NFR-3`).
- [ ] GCS buckets in a **`europe-*`** / EU multi-region location.
- [ ] Cloud Run, Cloud Tasks, Cloud Scheduler, Secret Manager, Artifact Registry all in **`europe-*`**.
- [ ] Email provider configured for its **EU region/data centre** (`NFR-3`).
- [ ] LLM provider: **Anthropic DPA** in place; confirm data-processing terms and that no training on our data; document the cross-border transfer basis (note: Anthropic/Gemini API endpoints are a documented sub-processor — record in the RoPA/DPA register). M0 Gemini is demo-only.
- [ ] WhatsApp Cloud API / Meta sub-processor documented in the DPA register.
- [ ] Cloudflare configured to keep EU traffic in-region where applicable.

### 7.6 Data-subject rights
- [ ] **Access:** export a subject's personal data (contact, consents, their leads + transcripts).
- [ ] **Erasure:** the cascade procedure (§7.4).
- [ ] **Portability:** provide the subject's data in a structured, machine-readable format (JSON) on request.
- [ ] **Rectification / objection / consent withdrawal:** supported and audited.

---

## 8. Audit log event catalog (`activity_logs`)

Schema (`DATA_MODEL.md`): `actor_type` (agent/staff/provider/system) · `actor_id` · `action` · `entity_type` · `entity_id` · `meta` (jsonb) · `at`.
**Rule:** log every meaningful action; **no PII in `meta` beyond what is strictly necessary** (`CLAUDE.md` §5). Prefer IDs over raw personal data.

Events that **MUST** be logged:

| action | actor_type | entity_type | entity_id | meta (minimal) |
|---|---|---|---|---|
| `company_verification` | agent/system | company | company_id | CUI, source=demoanaf, vat_status, checked_at |
| `lead_created` | agent | lead | lead_id | vertical, intent, source, ip, timestamp |
| `consent_given` | agent | consent | consent_id | text_version, channel, ip |
| `consent_withdrawn` | staff/system | consent | consent_id | text_version |
| `file_uploaded` | agent/provider | document | document_id | kind, mime, owner_type, size |
| `handoff_requested` | agent | lead | lead_id | reason (on-task/confusion/verify-fail) |
| `handoff_claimed` | staff | lead | lead_id | staff_id |
| `offer_status_changed` | staff | lead/offer | id | from_status, to_status, value |
| `lead_assigned` | staff/admin | lead | lead_id | assigned_to |
| `match_routed` [P2] | system | match | match_id | provider_id, rank, attempt_no |
| `match_accepted` / `match_declined` [P2] | provider | match | match_id | provider_id |
| `lead_terms_accepted` (clickwrap) [P2] | provider | lead | lead_id | terms_version, ip, user_id |
| `client_data_disclosed` [P2] | provider/system | lead | lead_id | provider_id (anti-circumvention, `FR-9.8`) |
| `offer_uploaded` [P2] | provider | offer | offer_id | provider_id, document_id, value |
| `magic_link_accessed` [P2] | system | lead | lead_id | jti, ip, action=view/accept/clarify |
| `data_access_export` (DSAR) | admin | contact | contact_id | subject_id |
| `data_erasure` (DSAR) | admin/system | contact | contact_id | subject pseudonymous id |
| `secret_rotated` | system | secret | secret_name | rotation only — never the value |
| `webhook_signature_rejected` | system | webhook | — | source=whatsapp, reason (no body) |
| `auth_failure` | system | user | — | route, ip (rate-limit/abuse signal) |

- [ ] Audit log is append-only in spirit; erasure **redacts** (§7.4) rather than deletes the trail.
- [ ] No secrets, no full transcripts, no raw card/IBAN-type data ever in `meta`.

---

## 9. Transport & infrastructure security

### 9.1 TLS
- [ ] HTTPS/TLS on all endpoints (Cloud Run + Cloudflare); HSTS header.
- [ ] DB connections use TLS (`sslmode=require` — already in the demo `DATABASE_URL` template).
- [ ] All outbound calls (LLM, WhatsApp, DemoANAF, email) over TLS.

### 9.2 Cloudflare WAF / edge
- [ ] Cloudflare in front of all public hostnames (`ARCHITECTURE.md` §6): TLS termination, WAF rules, rate limiting, bot/DDoS protection.
- [ ] Widget served via Cloudflare CDN; origin not directly exposed.

### 9.3 Least-privilege IAM
- [ ] **One dedicated service account per Cloud Run service**, granted only the roles it needs (Secret Manager accessor for its own secrets, Cloud SQL client, GCS object access on its bucket, Cloud Tasks enqueuer).
- [ ] No use of default/over-broad service accounts.
- [ ] Prefer Workload Identity over exported SA key files (§2).
- [ ] Separate GCP project per environment (`CLAUDE.md` §6).

### 9.4 CORS — **FIX THE CURRENT `*`**
**Finding (M0 gap):** CORS is wildcard `*` in two places:
- `backend/vercel.json` → `Access-Control-Allow-Origin: *`
- `backend/pkg/adapters/http/response.go` → `setCORS()` sets `Access-Control-Allow-Origin: *`

- [ ] **Replace `*` with an explicit allow-list** of known origins: the dashboard origin, the hosted chat page, and any approved widget-embed origins (the widget is embeddable, so maintain a configurable allow-list rather than `*`).
- [ ] Drive allowed origins from config/env, not hardcoded.
- [ ] Only echo back an `Origin` that is on the allow-list; otherwise omit the CORS header.
- [ ] Keep `Allow-Methods`/`Allow-Headers` minimal (only what the API uses).
- [ ] Fix **both** `vercel.json` and the Go `setCORS()` helper consistently.

### 9.5 Security headers
- [ ] `Strict-Transport-Security` (HSTS)
- [ ] `Content-Security-Policy` (constrain script/connect/img sources; needed for XSS defense §6.3 and for the embeddable widget)
- [ ] `X-Content-Type-Options: nosniff`
- [ ] `X-Frame-Options` / `frame-ancestors` (dashboard not embeddable; widget host explicitly allow-listed)
- [ ] `Referrer-Policy: strict-origin-when-cross-origin`

---

## 10. CI security

- [ ] **Secret scanning** in CI (e.g. gitleaks / GitHub secret scanning) on every push and PR — blocks merges that introduce secrets; would have caught a committed `.env`.
- [ ] **Dependency scanning** — Go modules (`govulncheck` / Dependabot) and npm (`npm audit` / Dependabot) for known CVEs; fail/alert on high severity.
- [ ] **SAST** — static analysis on Go (`gosec`, `go vet`, staticcheck) and the frontend (eslint security rules, `tsc` strict) in CI.
- [ ] **No deploy from a laptop** — build and deploy only from CI (`CLAUDE.md` §6); CI authenticates to GCP via Workload Identity Federation, not exported keys.
- [ ] **Container image scanning** in Artifact Registry (vulnerability scanning) before deploy.
- [ ] **Migrations run as part of deploy**, not manually (`CLAUDE.md` CI/CD expectations).
- [ ] CI secrets stored in the CI secret store, never echoed in logs.

---

## Appendix A — M0 hardening backlog (current known gaps)

Prioritized list of demo-state security debt to close before/at production:

1. [ ] **Rotate the live Gemini API key** in `backend/.env` (assume exposed) and any Neon DB credential (§2.4).
2. [ ] **Add authentication** to the dashboard API (`/api/leads`, detail) — currently unauthenticated (§1.3, §3).
3. [ ] **Lock down CORS** — replace `*` in `backend/vercel.json` and `backend/pkg/adapters/http/response.go` with an origin allow-list (§9.4).
4. [ ] **Add the WhatsApp webhook with signature verification** when the channel ships (§4).
5. [ ] **Add rate limiting + body-size caps** on the public chat endpoint (§1.1).
6. [ ] **Add security headers + CSP** (§9.5).
7. [ ] **Wire `activity_logs`** for the M0 events already in scope (verification, lead creation, consent) (§8).
8. [ ] **Stand up secret + dependency scanning in CI** (§10).
9. [ ] **Confirm gitignore + clean git history** for all `.env` files (§2.4).
