# API_CONTRACT.md — Euro Intermed B2B Platform REST API

> Companion to `docs/specs/openapi.yaml` (machine-readable). This file is the
> human-readable contract: conventions, auth, error shape, pagination, rate
> limits, validation, and per-endpoint behaviour. Read `CLAUDE.md`,
> `ARCHITECTURE.md`, `DATA_MODEL.md`, and `REQUIREMENTS.md` first.

Phase tags: **[P1]** Phase 1 MVP · **[P1-demo]** the Milestone-0 demo slice
(already implemented) · **[P2]** Phase 2 SkalYou (designed now, built later).

---

## 1. Conventions

### 1.1 Base URL

- All base URLs come from configuration; **never hardcode**.
  - Frontend reads `import.meta.env.VITE_API_URL` (Vite). The demo client
    (`frontend/src/lib/api.ts`) already does `const API_URL = import.meta.env.VITE_API_URL ?? ''`.
  - In production the backend is a Cloud Run service behind Cloudflare; in the
    demo it is a Vercel deployment. The OpenAPI `servers` block lists templated
    URLs only — clients substitute their own.
- API is rooted at `/api`. All paths in this document are relative to the base.
- Content type is `application/json; charset=utf-8` for every JSON body.
- Times are RFC 3339 / ISO 8601 UTC strings (e.g. `2026-06-21T09:25:00Z`).
- IDs are opaque strings (UUIDs in production). Treat them as opaque.

### 1.2 Versioning

The API is unversioned at the path level for Phase 1 (single consumer set we
control). Breaking changes are additive-first per the data-model rules. If a
hard break is ever needed, introduce `/api/v2`. Do not break `/api/chat`,
`/api/leads`, or `/api/leads/{id}` — the demo frontend depends on them as-is.

### 1.3 Correlation

Every response carries `X-Request-Id` (echoes the inbound `X-Request-Id` if the
client sent one, otherwise server-generated). Include it in bug reports; it maps
to Cloud Logging + Sentry (NFR-7). Agent turns additionally log a
`conversation_id` correlation field.

---

## 2. Authentication & authorization

Four distinct auth surfaces. Each endpoint section states which applies.

| Surface | Mechanism | Header / transport | Used by |
|---|---|---|---|
| **Public** | none | — | chat widget, WhatsApp webhook (signature, not auth), GDPR erasure intake |
| **Staff dashboard** [P1] | Session **JWT** (Bearer) | `Authorization: Bearer <jwt>` | dashboard SPA |
| **Provider portal** [P2] | OAuth (Google) **or** email **OTP** → provider JWT | `Authorization: Bearer <jwt>` | SkalYou provider portal |
| **Client offer page** [P2] | **Magic-link JWT** (single-purpose, expiring, no account) | token in URL `?t=` → exchanged for short-lived `Authorization: Bearer <jwt>` | client offer view |

### 2.1 Staff JWT [P1]

- Obtained via `POST /api/auth/login` (email + password) → returns a signed JWT
  (HS256/RS256) with claims `sub` (user id), `role` (`staff` | `admin`),
  `exp`, `iat`. Short TTL (e.g. 1h) + refresh via `POST /api/auth/refresh`.
- Sent as `Authorization: Bearer <jwt>` on every dashboard call.
- **RBAC:** `staff` can read/work leads, companies, listings, handoff, KPIs,
  activity. `admin` adds user management and GDPR erasure execution. Endpoints
  note the minimum role. Missing/expired token → `401`; insufficient role →
  `403`.

### 2.2 Provider auth [P2]

- `POST /api/provider/auth/google` (Google ID token) **or** the two-step OTP:
  `POST /api/provider/auth/otp/request` → `POST /api/provider/auth/otp/verify`.
  Both yield a provider JWT (`role: provider`, `provider_id` claim).
- Provider endpoints are **scoped to the authenticated provider** — a provider
  can only ever see its own leads/offers/matches (FR-9.2, FR-9.8). The server
  enforces this; never trust a `provider_id` from the body.

### 2.3 Client magic-link [P2]

- Staff/cron issues a magic link containing a single-use, expiring JWT
  (`aud: client-offer`, `offer_id`, `exp`, `jti`). Link is emailed to the client.
- The client page calls `POST /api/client/session` with the raw token to
  exchange it for a short-lived bearer JWT (so the long-lived token isn't put on
  every request). Every exchange and every data view is **access-logged**
  (FR-9.4, NFR-2). Expired/used → `410 Gone`.

### 2.4 Webhook signature (not auth, but security) [P1]

WhatsApp inbound (`POST /api/webhooks/whatsapp`) is verified by
`X-Hub-Signature-256` (HMAC-SHA256 of the raw body using the app secret). The
GET verification handshake uses `hub.verify_token`. See §5.

---

## 3. Error envelope

All non-2xx responses (except the bare `200` fast-ack webhook and the SSE/WS
streams) use one envelope:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "message is required",
    "details": [
      { "field": "message", "issue": "required" }
    ],
    "request_id": "req_01H...."
  }
}
```

- `code` — stable, machine-readable `SCREAMING_SNAKE_CASE`. Clients branch on
  `code`, never on `message`.
- `message` — human-readable, safe to surface (no secrets, no PII beyond what
  the caller already holds).
- `details` — optional array; for validation errors it lists `{field, issue}`.
- `request_id` — mirrors `X-Request-Id` for support correlation.

> **Compatibility note:** the three already-shipped demo handlers
> (`/api/chat`, `/api/leads`, `/api/leads/{id}`) currently emit the legacy shape
> `{ "error": "<string>" }`. The contract target is the structured envelope
> above; new endpoints MUST use it, and the demo handlers SHOULD migrate. Both
> shapes are documented in OpenAPI so the spec lints against reality today and
> the target tomorrow.

### 3.1 Standard status codes

| Status | When |
|---|---|
| `200 OK` | successful read / turn / mutation returning a body |
| `201 Created` | resource created (e.g. erasure request accepted into queue with a body) |
| `202 Accepted` | accepted for async processing (agent turn enqueued, erasure scheduled) |
| `204 No Content` | success, no body (CORS preflight, some PATCH) |
| `400 Bad Request` | malformed JSON / validation failure (`VALIDATION_FAILED`) |
| `401 Unauthorized` | missing/invalid/expired token (`UNAUTHENTICATED`) |
| `403 Forbidden` | authenticated but role/ownership denies (`FORBIDDEN`) |
| `404 Not Found` | resource does not exist (`NOT_FOUND`) |
| `405 Method Not Allowed` | wrong verb on a known path |
| `409 Conflict` | invalid state transition (e.g. illegal offer-status change) (`CONFLICT`) |
| `410 Gone` | expired/used magic link or clickwrap window (`EXPIRED`) |
| `422 Unprocessable Entity` | semantically invalid (e.g. unknown enum value) (`UNPROCESSABLE`) |
| `429 Too Many Requests` | rate limited (`RATE_LIMITED`) — see §6 |
| `500 Internal Server Error` | unexpected (`INTERNAL`) — never leaks internals |
| `502/503` | upstream (LLM, DemoANAF, WhatsApp, email) unavailable (`UPSTREAM_UNAVAILABLE`) |

---

## 4. Pagination

**Convention: cursor-based** (`limit` + opaque `cursor`). Justification:

- Lead/company/listing tables grow and are sorted by `created_at desc`; offset
  pagination drifts and double-counts when new rows arrive mid-scroll. Cursors
  are stable under inserts.
- It keeps the dashboard's "load more / infinite scroll" cheap and index-friendly
  (keyset on `(created_at, id)`).

Request:

```
GET /api/leads?limit=25&cursor=eyJjcmVhdGVkX2F0Ijoi..."
```

- `limit` — default `25`, max `100`. Out-of-range → clamped (not an error).
- `cursor` — opaque base64 token from the previous page's `next_cursor`. Omit
  for the first page. An invalid/garbled cursor → `400 VALIDATION_FAILED`.

Response envelope for **all** collection endpoints:

```json
{
  "data": [ /* items */ ],
  "page": {
    "next_cursor": "eyJ...",   // null when there are no more pages
    "limit": 25,
    "count": 25                 // items in this page
  }
}
```

> **Demo exception:** the shipped `GET /api/leads` returns a **bare array**
> (the demo client expects `Lead[]`). The contract target is the paginated
> envelope; the OpenAPI documents the bare-array response for the current demo
> operation and the envelope for the P1 dashboard operation. New collection
> endpoints MUST use the envelope.

---

## 5. Real-time channels (chat transport)

The agent core is channel-agnostic (FR-1.6); the web widget speaks one of two
transports. Both deliver the same logical events; the typing indicator and
streamed reply are first-class (FR-1.3).

### 5.1 Turn request (HTTP) [P1, P1-demo]

`POST /api/chat` is the synchronous fallback and the demo path. Body:
`{ "conversation_id"?: string, "message": string }`. If `conversation_id` is
omitted, the server creates a conversation (channel `web`) and returns its id.
Response: `{ conversation_id, reply, state, extracted }`. In production the turn
is enqueued (Cloud Tasks) and the HTTP response may be the acknowledged state
while the streamed reply arrives over SSE/WS; in the demo it is synchronous.

### 5.2 Session bootstrap [P1]

`POST /api/widget/session` issues an ephemeral widget session (so an embedded
`<script>` widget on a third-party site can open a stream without staff auth).
Body: `{ "vertical"?: "angrosist"|"palletclearance", "locale"?: "ro"|"en",
"origin": string }`. Returns `{ session_id, conversation_id, stream_url,
expires_at }`. `stream_url` already encodes the transport and a short-lived
session token. CORS: the widget is cross-origin by design; `origin` is recorded
for abuse tracing.

### 5.3 SSE stream [P1] — recommended default

`GET /api/chat/stream?conversation_id={id}&session={token}`
`Accept: text/event-stream`.

- Server keeps the connection open and emits named events:

  | event | data (JSON) | meaning |
  |---|---|---|
  | `typing` | `{ "on": true \| false }` | typing indicator on/off |
  | `token` | `{ "text": "..." }` | incremental reply token (streamed LLM output) |
  | `message` | `{ "id", "role":"assistant", "content", "state", "extracted" }` | a completed assistant turn |
  | `state` | `{ "state": "qualifying" \| ... }` | flow-state change |
  | `handoff` | `{ "needs_human": true }` | escalation occurred (FR-6.1) |
  | `error` | `{ "code", "message" }` | recoverable stream error |
  | `ping` | `{}` | keep-alive (every ~15s) |

- Reconnection: standard SSE `Last-Event-ID` header resumes after the last
  delivered event id. The client posts user messages via `POST /api/chat`
  (with the same `conversation_id`) and reads replies on the stream.

### 5.4 WebSocket stream [P1] — alternative

`GET /api/chat/ws?session={token}` (Upgrade: websocket). Cloud Run supports
WebSocket. JSON frames both ways:

- Client → server: `{ "type": "message", "conversation_id", "text" }`
  and `{ "type": "typing", "on": true }` (user-typing, optional).
- Server → client: frames mirror the SSE events above, each as
  `{ "type": "typing"|"token"|"message"|"state"|"handoff"|"error"|"pong", ... }`.
- Heartbeat: server `ping` / client `pong` every ~20s; idle sockets closed at
  ~60s. Backpressure: server may coalesce `token` frames.

> Pick **one** transport per deployment and keep it consistent (ARCHITECTURE
> §"pick one"). SSE is the default (simpler, proxy-friendly through Cloudflare);
> WS is documented for live-takeover (FR-6.3) later.

---

## 6. Rate limiting

Public surfaces (chat, widget session, webhook GET verify, GDPR intake, client
magic-link exchange, provider OTP request) are rate limited per IP + per
conversation/session. Every rate-limited response (and, best-effort, successful
ones on limited routes) carries:

| Header | Meaning |
|---|---|
| `RateLimit-Limit` | requests allowed in the current window |
| `RateLimit-Remaining` | requests left in the window |
| `RateLimit-Reset` | seconds until the window resets |
| `Retry-After` | (on `429` only) seconds to wait before retrying |

`429` body uses the error envelope with `code: "RATE_LIMITED"`. OTP request and
magic-link exchange have stricter buckets to deter enumeration/abuse (NFR-2).

---

## 7. Input validation (everywhere)

Validation is mandatory on every write and on every query parameter. General
rules; per-endpoint specifics live in the endpoint tables and the OpenAPI schema
(`required`, `minLength`, `maxLength`, `enum`, `format`, `pattern`).

- **Reject unknown/malformed JSON** → `400 VALIDATION_FAILED`.
- **String bounds:** `message` 1–4000 chars; names ≤ 200; free-text notes ≤ 2000.
- **Enums** validated against the *current* extensible set; unknown value →
  `422 UNPROCESSABLE` (never silently coerced). Verticals/intents/statuses are
  extensible — validate against the live set, do not hardcode "exactly two".
- **CUI** (Romanian fiscal code): digits only, optional `RO` prefix, length
  2–10; validated before any DemoANAF call (FR-4.1). Server strips `RO`/spaces.
- **Email** `format: email`; **phone** E.164-ish pattern; **country**
  ISO 3166-1 alpha-2.
- **Pagination:** `limit` integer 1–100 (clamped), `cursor` must decode.
- **IDs** in paths must be non-empty and well-formed, else `400`/`404`.
- **Money/quantity:** non-negative numbers; `quantity` may be fractional;
  `value`/`budget` ≥ 0.
- **Clickwrap/consent:** boolean `accepted` must be `true` to proceed; the
  server stamps `terms_version`, `ip`, `timestamp`, `user_id` — these are never
  taken from the client (FR-9.3).
- **Webhook:** body must verify against `X-Hub-Signature-256` before parsing;
  failure → `403` (and the request is dropped).

---

## 8. Endpoints

### 8.1 Public chat / agent [P1]

| Method & path | Auth | Phase | Notes |
|---|---|---|---|
| `POST /api/chat` | none | P1-demo | Run one agent turn. Body `{conversation_id?, message}`. `message` required, 1–4000 chars. Returns `{conversation_id, reply, state, extracted}`. Creates a conversation when `conversation_id` absent. |
| `POST /api/widget/session` | none | P1 | Bootstrap an embeddable-widget session. Body `{vertical?, locale?, origin}`. Returns `{session_id, conversation_id, stream_url, expires_at}`. |
| `GET /api/chat/stream` | session token (query) | P1 | SSE. Query `conversation_id`, `session`. Emits `typing|token|message|state|handoff|error|ping` (see §5.3). |
| `GET /api/chat/ws` | session token (query) | P1 | WebSocket alternative (see §5.4). |

`state` is one of the implemented conversation states:
`greeting | qualifying | verifying | confirmed | failed`. `extracted` is a free
map (demo: `product_name, quantity, unit, delivery_location, cui, company_name,
phone, email`). Validation: `vertical` enum, `locale ∈ {ro,en}`, `origin` a URL.

### 8.2 WhatsApp webhook [P1]

| Method & path | Auth | Phase | Notes |
|---|---|---|---|
| `GET /api/webhooks/whatsapp` | verify token | P1 | Meta subscription handshake. Query `hub.mode=subscribe`, `hub.verify_token`, `hub.challenge`. If `hub.verify_token` matches the configured secret, return **`hub.challenge` verbatim as `text/plain` 200**; else `403`. |
| `POST /api/webhooks/whatsapp` | `X-Hub-Signature-256` | P1 | Signed inbound events. **Verify HMAC-SHA256 of the raw body with the app secret before parsing** (`403` on mismatch). On valid: **fast-ack `200` immediately**, dedupe by provider message id, enqueue the turn (Cloud Tasks). Never block on the LLM (FR-1.4, FR-2.3). Body is Meta's webhook payload (messages/statuses). |

Fast-ack semantics: the `200` means "received", not "processed". Reprocessing is
idempotent via the message-id dedupe key. Retries from Meta are expected and
safe.

### 8.3 Dashboard API [P1, authenticated]

All require `Authorization: Bearer <staff-jwt>`. Min role noted. Collections use
the §4 paginated envelope unless flagged demo.

| Method & path | Min role | Phase | Notes |
|---|---|---|---|
| `POST /api/auth/login` | — | P1 | Body `{email, password}` → `{token, expires_at, user}`. Rate limited. |
| `POST /api/auth/refresh` | staff | P1 | Rotate a near-expiry JWT. |
| `GET /api/leads` (demo) | — (demo) / staff (P1) | P1-demo / P1 | **Demo:** bare `Lead[]`, no auth, no paging (current behaviour). **P1:** paginated + filtered + authed (operation `listLeads`). |
| `GET /api/leads` (P1) | staff | P1 | Filters: `status`, `vertical`, `assigned_to`, `country`/`market`, `search` (full-text over company/product/summary). Plus `limit`,`cursor`. Returns `{data: LeadSummary[], page}`. |
| `GET /api/leads/{id}` | staff | P1 | Lead detail: summary + `address, county, phone, email`, **`transcript[]`**, extracted fields, and the **typed request** (`sourcing_request` \| `listing` \| [P2] `market_entry_request`). `404` if absent. |
| `PATCH /api/leads/{id}/status` | staff | P1 | Manual offer/lead status. Body `{status, value?, note?}`. `status` must be a valid transition in the lead state machine (`new→qualifying→…→won/lost`, plus `offer_requested→offer_sent→negotiation`); illegal transition → `409 CONFLICT`. Writes `activity_logs`. |
| `PATCH /api/leads/{id}/assign` | staff | P1 | Assign/unassign. Body `{assigned_to: userId \| null}`. `404` if user or lead missing. |
| `GET /api/leads/{id}/activity` | staff | P1 | Activity/audit log for the lead (paginated). Items `{actor_type, actor_id, action, meta, at}`. |
| `GET /api/handoff` | staff | P1 | Handoff queue: leads with `needs_human=true` / `bot_active=false`, newest first (FR-6.2). Paginated, same filters as leads. |
| `GET /api/companies` | staff | P1 | B2B directory list. Filters: `roles` (any-of), `country`, `vat_status`, `search` (name/reg_no). Paginated. |
| `GET /api/companies/{id}` | staff | P1 | Company detail incl. `roles[]`, latest verification, optional financials, linked contacts & leads. |
| `GET /api/listings` | staff | P1 | PalletClearance inventory. Filters: `status`, `category`, `country`, `confidential`. Paginated. |
| `GET /api/kpis` | staff | P1 | Aggregate KPIs: `offers_sent`, `conversion_rate`, `pipeline_value`, counts by status/vertical. Optional `from`/`to` window. |

`LeadSummary` (matches `domain.LeadSummary`): `id, status, company_name, cui,
product_name, quantity (nullable), unit, delivery_location, created_at`.
`LeadDetail` adds `address, county, phone, email, transcript[]` and the typed
request object. `transcript[]` items: `id, role, content, tool_calls?
(base64 JSON), created_at`.

### 8.4 GDPR [P1]

| Method & path | Auth | Phase | Notes |
|---|---|---|---|
| `POST /api/gdpr/erasure` | none (intake) | P1 | Data-subject erasure request. Body `{email? , phone?, contact_id?, reason?}` — at least one identifier required (`422` otherwise). Returns `202` with a `request_id`. Triggers **cascade erasure** contact→lead→conversation→files once verified (NFR-3). |
| `POST /api/gdpr/erasure/{id}/execute` | admin | P1 | Admin confirms/executes a verified erasure request. Writes an audit entry; idempotent. |

### 8.5 Provider portal [P2]

All P2. Provider JWT required except the auth endpoints. Every endpoint is
scoped to the authenticated provider.

| Method & path | Auth | Notes |
|---|---|---|
| `POST /api/provider/auth/google` | none | Google ID token → provider JWT. |
| `POST /api/provider/auth/otp/request` | none | Body `{email}` → sends OTP. Strict rate limit. |
| `POST /api/provider/auth/otp/verify` | none | Body `{email, code}` → provider JWT. |
| `POST /api/provider/onboarding` | provider | Profile: `categories[], roles[], markets[], plan, consent_to_leads`. `consent_to_leads` must be `true`. |
| `GET /api/provider/leads` | provider | Provider's routed leads. **Each lead's full client data is gated by clickwrap** — until accepted, only a redacted teaser is returned (FR-9.3, FR-9.8). |
| `POST /api/provider/matches/{id}/accept` | provider | Accept a routed match. `409` if already responded/expired. Logs acceptance + timestamp. |
| `POST /api/provider/matches/{id}/decline` | provider | Decline; advances matching to next provider. |
| `POST /api/provider/leads/{id}/clickwrap` | provider | **Clickwrap accept** before client-data disclosure. Body `{accepted: true}`. Server records `terms_version, ip, timestamp, user_id` (never from client). Returns the full client data. `410` if window expired. |
| `POST /api/provider/offers` | provider | Upload an offer. Multipart or pre-signed-URL flow; body refs `lead_id`/`request_id`, `value`, and the offer document (GCS). Photos/docs always to GCS. |

### 8.6 Client offer page (magic-link) [P2]

All P2. No account; access via expiring, access-logged magic link.

| Method & path | Auth | Notes |
|---|---|---|
| `POST /api/client/session` | magic-link token (body) | Exchange the raw link token for a short-lived client JWT. Expired/used → `410`. Logs the access. |
| `GET /api/client/offer` | client JWT | View the offer (and provider plan + value). Access logged. |
| `POST /api/client/offer/accept` | client JWT | Accept the offer → status `accepted`, stamps `client_action_at`. Idempotent; `409` if not in an acceptable state. |
| `POST /api/client/offer/clarifications` | client JWT | Request clarifications. Body `{message}` (1–2000 chars) → status `clarification_requested` + staff/provider notification (FR-9.4). |

---

## 9. CORS

Browser-facing endpoints set permissive CORS for the widget (cross-origin by
design) and the dashboard origin. Preflight `OPTIONS` returns `204` with
`Access-Control-Allow-{Origin,Methods,Headers}` (the demo handlers already do
this via `httputil.HandleOptions`). Production tightens `Allow-Origin` to the
known dashboard + widget-host origins rather than `*` where credentials are sent.

---

## 10. Summary of phase coverage

- **[P1-demo] live today:** `POST /api/chat`, `GET /api/leads` (bare array),
  `GET /api/leads/{id}`, `GET /api/health`.
- **[P1] to build:** widget session + SSE/WS, WhatsApp webhook (GET+POST),
  auth, full dashboard (filters/pagination/RBAC), status/assign, handoff,
  companies, listings, KPIs, activity, GDPR erasure.
- **[P2] designed now:** provider auth/onboarding/leads/matches/offers with
  clickwrap gating, and client magic-link view/accept/clarifications.
