# ARCHITECTURE_DETAIL.md — Ports, Adapters & the Swap Recipe

> Handover-grade detail for the Euro Intermed B2B platform. This document expands
> `docs/ARCHITECTURE.md` with **concrete Go port signatures**, the **target package
> layout**, the **async agent-turn sequence**, and — most importantly — the
> **swap recipe**: exactly how to add or replace an adapter (email, LLM, or a new
> channel/interface for the AI agent).
>
> Companion docs (read for context): `docs/CLAUDE.md`, `CLAUDE.md` (root),
> `docs/REQUIREMENTS.md`, `docs/DATA_MODEL.md`, `docs/ARCHITECTURE.md`.
>
> Tech stack is **locked** (Go · Cloud SQL/Postgres · Cloud Tasks · Cloud Run ·
> GCS · Secret Manager · Claude LLM, Gemini for demo · WhatsApp Cloud API ·
> DemoANAF). Do not propose substitutions; propose *new adapters behind the same
> ports*.

---

## 1. The hexagonal principle and the dependency rule

The system is **hexagonal (ports & adapters)**. There are three concentric layers:

```
        ┌──────────────────────── adapters (infrastructure) ────────────────────────┐
        │  HTTP handlers · WhatsApp · web widget · Postgres · GCS · DemoANAF ·       │
        │  email provider · Cloud Tasks · Claude/Gemini SDKs · clock · uuid          │
        │   ┌──────────────────────── application (use-cases) ────────────────────┐  │
        │   │  agent turn orchestration, lead creation, verification, handoff     │  │
        │   │   ┌──────────────────────── domain (core) ──────────────────────┐   │  │
        │   │   │  entities + invariants: Company, Contact, Lead,             │   │  │
        │   │   │  Conversation, Message, SourcingRequest, Listing, Consent…  │   │  │
        │   │   └─────────────────────────────────────────────────────────────┘   │  │
        │   └──────────────────────────────────────────────────────────────────────┘  │
        └────────────────────────────────────────────────────────────────────────────┘
```

**The dependency rule (the one rule everything else serves):**

> **Source-code dependencies point inward only.** The domain imports nothing
> external — no HTTP, no SQL driver, no Anthropic/Gemini SDK, no GCS client, no
> WhatsApp client, no `net/http`, no `os`. Everything outside the domain is reached
> through a **port** (a Go `interface` owned by the inner layer) and satisfied by
> exactly one production **adapter** (in the outer layer).

Concretely, in Go terms:

- **Ports are interfaces declared by the consumer** (domain/application), *not* by
  the adapter. The application says "I need a `Mailer`"; the `internal/email`
  package provides a type that *happens to* satisfy it. This is idiomatic Go:
  accept interfaces, return structs.
- The LLM **never** holds a DB handle or an API key. It emits **tool calls**; our
  code executes those tool calls against ports. (Today the demo wires
  `verify_company` and `save_lead` directly in the Gemini runner — M1 moves tool
  execution into the agent core so it is LLM-vendor-agnostic; see §3 LLM port and
  §4.)
- **Dependency direction is enforced by package boundaries.** `internal/domain`
  must compile with zero imports of `internal/persistence`, `internal/channels`,
  `internal/agent`, etc. A CI check (`go list -deps` / an import-linter rule)
  fails the build if the domain imports an adapter package.
- **Wiring happens once, at the edge** (`/cmd/*` via a small composition root /
  container). Adapters are constructed there and injected as interfaces. Swapping
  an adapter is a change in one wiring file, never in the domain.

Why we care (from `REQUIREMENTS.md` NFR-8 and `CLAUDE.md`): the strategic asset is
the normalized B2B company database, and the agent must run on **many channels**.
Both demand that vendors (LLM, channel, email, storage) be swappable without
touching qualification logic.

---

## 2. Target Go layout and how the demo maps into it

### 2.1 Target layout (what we refactor INTO during M1)

This is the layout declared in `CLAUDE.md` §4. M1 is the milestone that performs
the refactor from the demo's `backend/pkg/...` into `backend/internal/...`.

```
backend/
  cmd/
    api/         # HTTP server entrypoint (webhook, widget WS/SSE, dashboard API)
    worker/      # Cloud Tasks worker entrypoint (the async agent turn)
    migrate/     # DB migration runner
  internal/
    domain/        # entities + invariants + use-case interfaces. NO external imports.
    agent/         # channel-agnostic agent: flow engine, tool registry, turn orchestration.
                   #   depends on the LLM port + repo ports + service ports, never on a vendor SDK.
    channels/      # inbound/outbound channel adapters (Channel port)
      whatsapp/    #   Meta Cloud API: signature verify, 24h window, templates
      webwidget/   #   WS/SSE sessions for the embeddable widget + hosted page
      (telegram/)  #   future channels drop in here — see §5(c)
    verification/  # DemoANAF adapter (CompanyDataProvider port)
    persistence/   # Postgres repositories (the Repository ports)
    storage/       # GCS adapter (FileStore port)
    email/         # email provider adapter (Mailer port)
    queue/         # Cloud Tasks adapter (Queue port)
    api/           # HTTP handler layer: routing, request/response, auth, DTO mapping
    config/        # env + Secret Manager loading (the only place secrets are read)
  migrations/      # forward-only SQL migrations
  deploy/          # Terraform, Dockerfile, CI workflows
web/               # frontend (dashboard, widget, [P2] provider portal, client page)
```

Notes:
- **Ports live with their consumer.** Repository interfaces, the `LLM` port, the
  `Channel` port, `Mailer`, `Queue`, `CompanyDataProvider`, `FileStore`, `Clock`,
  `IDGen` are declared in `internal/domain` (or a thin `internal/domain/ports`
  sub-package). Adapters in `internal/persistence`, `internal/email`, etc.
  *implement* them. This keeps the demo's "ports in one place" ergonomics while
  honoring the dependency rule.
- `cmd/api` and `cmd/worker` are **separate binaries from the same image**
  (selected by an entrypoint flag/env). Both build the same container; Cloud Run
  runs the API service and the Cloud Tasks push target.

### 2.2 Mapping from the current demo (`backend/pkg/...`)

The demo already follows ports-and-adapters; M1 is mostly a **move + rename**, not
a rewrite. The data model and SQL migrations are preserved (additive only).

| Demo (today) | Target (M1) | Action |
|---|---|---|
| `backend/pkg/domain/{company,contact,lead,sourcing,conversation}.go` | `internal/domain/` | move; keep entities; add invariants as methods |
| `backend/pkg/ports/repositories.go` (`ConversationRepo`, `MessageRepo`, `LeadRepo`, `CompanyRepo`, `ContactRepo`, `SourcingRepo`) | `internal/domain` (ports) | move; **widen** to full aggregates (§3) — add `DocumentRepo`, `ConsentRepo`, `ActivityLogRepo`, `UserRepo` |
| `backend/pkg/ports/services.go` (`CompanyVerifier`, `AgentRunner`) | `internal/domain` (ports) | rename `CompanyVerifier` → `CompanyDataProvider`; keep `AgentRunner` as the agent entrypoint |
| `backend/pkg/adapters/postgres/*` | `internal/persistence/` | move; one repo file per aggregate |
| `backend/pkg/adapters/anaf/*` | `internal/verification/` | move; implements `CompanyDataProvider` |
| `backend/pkg/adapters/gemini/*` | split: `internal/agent/` (flow + tools) + `internal/agent/llm/gemini/` (LLM adapter) | **split** — see below |
| `backend/pkg/adapters/http/*`, `backend/api/*` | `internal/api/` + `cmd/api` | move handlers; consolidate routing |
| `backend/pkg/app/container.go` | `cmd/api` + `cmd/worker` composition roots | move wiring; add worker wiring |
| `backend/pkg/usecases/{chat,leads}.go` | `internal/domain` use-cases / `internal/agent` | move |
| *(none yet)* | `internal/channels/{whatsapp,webwidget}/` | **new** in M1 |
| *(none yet)* | `internal/storage/` (GCS), `internal/email/`, `internal/queue/` (Cloud Tasks), `internal/config/` | **new** in M1 |

**The key refactor — splitting the Gemini runner.** Today
`backend/pkg/adapters/gemini/runner.go` does three jobs at once: (1) talks to the
Gemini SDK, (2) runs the conversation/flow loop, and (3) executes tools
(`verify_company`, `save_lead`) directly against repos. That couples our agent to
one LLM vendor. M1 separates them:

- **`internal/agent`** owns the turn loop, the flow engine, and the **tool
  registry** (tool execution against ports). It depends only on the `LLM` port and
  the repo/service ports.
- **`internal/agent/llm/gemini`** and **`internal/agent/llm/claude`** are thin
  adapters that satisfy the `LLM` port (`Complete` / `Stream` / tool-calling
  normalization). They contain *only* SDK glue — no repo access, no tool logic.

After this split, switching Claude↔Gemini is a one-line wiring change (§5b).

---

## 3. The ports (concrete Go interfaces)

All interfaces below are owned by the inner layers and live in `internal/domain`
(ports). Each has the responsibility stated, **exactly one production adapter**,
and is **mocked in tests** (table-driven unit tests for the agent/use-cases;
contract tests for each adapter against the real dependency where feasible).

Shared value types referenced below (illustrative, declared in `internal/domain`):

```go
package domain

import (
	"context"
	"io"
	"time"
)

// InboundMessage is the normalized form every channel adapter produces.
type InboundMessage struct {
	Channel          string            // "whatsapp" | "webwidget" | "telegram" | ...
	ProviderMsgID    string            // for dedupe/idempotency (WhatsApp wamid, etc.)
	ConversationRef  string            // channel-scoped conversation/thread key
	SenderRef        string            // channel-scoped sender (phone, session id, chat id)
	Text             string
	MediaRefs        []MediaRef        // attachments the channel exposes (URLs/handles)
	Lang             string            // detected/declared language, optional
	ReceivedAt       time.Time
	Raw              map[string]any    // provider-specific payload for audit
}

// OutboundMessage is what the agent asks a channel to deliver.
type OutboundMessage struct {
	ConversationRef string
	Text            string
	TemplateName    string            // for WhatsApp out-of-24h-window template sends
	TemplateArgs    map[string]string
	MediaRefs       []MediaRef
}

type MediaRef struct {
	ProviderHandle string // channel-side id to fetch the bytes
	MIME           string
	Filename       string
}
```

### 3.1 `Channel` — inbound normalization + outbound send

**Responsibility:** isolate one messaging surface (WhatsApp, web widget, future
Telegram/Instagram/voice). Verify inbound authenticity, **normalize** the
provider payload into `domain.InboundMessage`, and **send** outbound replies
(including channel-specific concerns like WhatsApp's 24h window/templates).

```go
package domain

// Channel is the single seam between the agent core and any messaging surface.
type Channel interface {
	// Name is the stable channel identifier stored on conversations/leads.
	Name() string

	// VerifyInbound authenticates a raw inbound request (e.g. WhatsApp
	// X-Hub-Signature-256). It must run BEFORE any processing. Returns a
	// terminal error if authenticity cannot be established.
	VerifyInbound(headers map[string]string, body []byte) error

	// Normalize converts a verified raw payload into zero or more normalized
	// inbound messages. (A single webhook can carry multiple events.)
	Normalize(body []byte) ([]InboundMessage, error)

	// Send delivers an outbound message through this channel.
	Send(ctx context.Context, msg OutboundMessage) error

	// FetchMedia resolves a channel-side media handle to bytes so the agent can
	// hand them to FileStore (e.g. seller photos in PalletClearance).
	FetchMedia(ctx context.Context, ref MediaRef) (io.ReadCloser, error)
}
```

**Production adapters:** `internal/channels/whatsapp` (Meta Cloud API) and
`internal/channels/webwidget` (WS/SSE). One adapter per surface; the agent core is
unchanged when a new one is added. **Mocked in tests** so the flow engine is tested
with a fake channel that records `Send` calls.

### 3.2 `LLM` — completion, streaming, tool-calling

**Responsibility:** the only seam to a language model. It exposes a **vendor-neutral**
request/response shape so Claude (production) and Gemini (demo) are interchangeable.
The LLM returns either assistant text or **tool calls**; our code executes the tools
(it never executes them itself).

```go
package domain

type LLM interface {
	// Complete runs one model call and returns the assistant turn (text and/or
	// tool calls). Tool *results* are fed back as follow-up Complete calls.
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)

	// Stream is the same call with token streaming, used by the web widget for
	// a typing/streaming UX. Tool calls are surfaced on the returned channel.
	Stream(ctx context.Context, req LLMRequest) (<-chan LLMChunk, error)
}

type LLMRequest struct {
	System   string         // system prompt (per vertical + language)
	Messages []LLMMessage   // running transcript, normalized
	Tools    []ToolSpec     // tool schemas the model may call
	Model    string         // resolved from config (e.g. claude-* / gemini-*)
	MaxTokens int
	Temperature float32
}

type LLMMessage struct {
	Role      string         // "user" | "assistant" | "tool"
	Text      string
	ToolCalls []ToolCall     // assistant-issued calls
	ToolResult *ToolResult   // result fed back for a prior call
}

type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any // JSON Schema of the arguments
}

type ToolCall struct {
	ID   string         // provider call id (for correlating results)
	Name string         // verify_company | upload_media | submit_lead | handoff_to_human | [P2] classify_need
	Args map[string]any
}

type ToolResult struct {
	CallID string
	Result map[string]any
	IsError bool
}

type LLMResponse struct {
	Text      string
	ToolCalls []ToolCall
	StopReason string         // "end_turn" | "tool_use" | "max_tokens" ...
}

type LLMChunk struct {
	TextDelta string
	ToolCall  *ToolCall
	Done      bool
	Err       error
}
```

**Production adapters:** `internal/agent/llm/claude` (Anthropic, Phase 1+) and
`internal/agent/llm/gemini` (`gemini-2.0-flash-lite`, demo only). Each maps our
`ToolSpec`/`ToolCall` to the vendor's tool-use format. **Mocked in tests** with a
scripted LLM that returns canned tool calls, so the flow engine and each tool are
tested without network or spend.

### 3.3 `CompanyDataProvider` — DemoANAF verification

**Responsibility:** verify a company by registration number and fetch optional
financials. Today's `CompanyVerifier` (`backend/pkg/ports/services.go`) renamed and
widened.

```go
package domain

type CompanyDataProvider interface {
	// Verify looks up a company by (country, regNo). regNo is the RO CUI today.
	// Returns nil + ErrCompanyNotFound for unknown; a *retryable* error if the
	// source is unavailable (so the worker can retry — see §6).
	Verify(ctx context.Context, country, regNo string) (*Company, error)

	// Financials is optional/nullable per company (foreign companies lack it).
	Financials(ctx context.Context, country, regNo string) (*CompanyFinancials, error)
}
```

**Production adapter:** `internal/verification` (DemoANAF `GET /api/company/:cui`,
`/financials`). Caches results into `companies` / `company_verifications` /
`company_financials`. **Mocked in tests** with a fake that returns active/inactive/
not-found/unavailable cases (the demo's anaf test already exercises this shape).

### 3.4 `FileStore` — GCS with signed URLs

**Responsibility:** store and serve binary documents (seller photos, buyer product
lists, [P2] offers). Files always live in GCS, never on instance disk
(`DATA_MODEL.md` rule 6).

```go
package domain

type FileStore interface {
	// Put streams an object to storage and returns its durable key.
	Put(ctx context.Context, key string, r io.Reader, contentType string) error

	// SignedGetURL returns a time-limited download URL (dashboard/portal access).
	SignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// SignedPutURL returns a time-limited upload URL for direct browser uploads.
	SignedPutURL(ctx context.Context, key, contentType string, ttl time.Duration) (string, error)

	// Delete removes an object (GDPR cascade erasure).
	Delete(ctx context.Context, key string) error
}
```

**Production adapter:** `internal/storage` (GCS, `europe-*` bucket). **Mocked in
tests** with an in-memory store; the `upload_media` tool is tested against it.

### 3.5 `Mailer` — templated transactional email

**Responsibility:** send templated RO/EN transactional email (lead confirmation to
prospect, internal staff notification) via the external provider.

```go
package domain

type Mailer interface {
	// Send renders a named template with data and delivers it. Template +
	// locale select the body; the adapter owns provider/template-id mapping.
	Send(ctx context.Context, msg EmailMessage) error
}

type EmailMessage struct {
	To       []string
	Template string            // "lead_confirmation" | "staff_notification" | ...
	Locale   string            // "ro" | "en"
	Data     map[string]any    // template variables
	ReplyTo  string
}
```

**Production adapter:** `internal/email` (SendGrid/Mailgun/Brevo, EU region — the
provider id and API key come from config/Secret Manager). **Mocked in tests** with
a recorder that asserts template + recipients.

### 3.6 `Queue` — Cloud Tasks enqueue

**Responsibility:** durably enqueue an agent-turn job for asynchronous processing.
The worker endpoint is the push target.

```go
package domain

type Queue interface {
	// Enqueue schedules a job for the worker. dedupeKey makes enqueue idempotent
	// (Cloud Tasks task name) so a redelivered webhook does not double-process.
	Enqueue(ctx context.Context, job AgentTurnJob, dedupeKey string) error
}

type AgentTurnJob struct {
	ConversationID string
	ProviderMsgID  string   // the inbound message id (dedupe inside the worker too)
	Channel        string
	Payload        []byte   // normalized InboundMessage, JSON
}
```

**Production adapter:** `internal/queue` (Cloud Tasks → push to `cmd/worker` on
Cloud Run). **Mocked in tests** with a synchronous fake that invokes the worker
in-process (lets us test ingest→worker end-to-end without GCP).

### 3.7 Repository ports — one interface per aggregate

**Responsibility:** persist and load each aggregate in Cloud SQL. Repos return
domain entities, never SQL rows. Migrations are additive (`DATA_MODEL.md` rule 8).
The demo has six repos; M1 adds the rest required by `DATA_MODEL.md` and the GDPR
requirements (`REQUIREMENTS.md` NFR-3). **All mocked in tests**; the production
adapters live in `internal/persistence` and get **integration tests against a real
Postgres**.

```go
package domain

type CompanyRepo interface {
	// Dedup key is (country, reg_no) — DATA_MODEL.md invariant 1.
	GetByRegNo(ctx context.Context, country, regNo string) (*Company, error)
	Upsert(ctx context.Context, c *Company) error
	AddRoles(ctx context.Context, companyID string, roles []string) error // opportunistic roles[]
	List(ctx context.Context, f CompanyFilter) ([]*Company, error)
}

type ContactRepo interface {
	Create(ctx context.Context, c *Contact) error
	Update(ctx context.Context, c *Contact) error
	GetByID(ctx context.Context, id string) (*Contact, error)
}

type LeadRepo interface {
	Create(ctx context.Context, l *Lead) error
	GetByID(ctx context.Context, id string) (*LeadDetail, error)
	GetByConversationID(ctx context.Context, convID string) (*Lead, error)
	UpdateCompanyContact(ctx context.Context, leadID, companyID, contactID string) error
	SetStatus(ctx context.Context, leadID string, status LeadStatus) error
	SetHumanFlags(ctx context.Context, leadID string, needsHuman, botActive bool) error
	List(ctx context.Context, f LeadFilter) ([]*LeadSummary, error)
}

type ConversationRepo interface {
	Create(ctx context.Context, channel, conversationRef string) (*Conversation, error)
	GetByID(ctx context.Context, id string) (*Conversation, error)
	GetByChannelRef(ctx context.Context, channel, conversationRef string) (*Conversation, error)
	UpdateState(ctx context.Context, id string, state ConversationState) error
	UpdateExtracted(ctx context.Context, id string, extracted map[string]any) error
}

type MessageRepo interface {
	Append(ctx context.Context, m *Message) error
	ListByConversation(ctx context.Context, conversationID string) ([]*Message, error)
	// SeenProviderMsg supports idempotency: true if this provider id was processed.
	SeenProviderMsg(ctx context.Context, channel, providerMsgID string) (bool, error)
	MarkProviderMsgSeen(ctx context.Context, channel, providerMsgID string) error
}

type DocumentRepo interface {
	Create(ctx context.Context, d *Document) error                       // polymorphic owner_type/owner_id
	ListByOwner(ctx context.Context, ownerType, ownerID string) ([]*Document, error)
	DeleteByOwner(ctx context.Context, ownerType, ownerID string) error  // GDPR cascade
}

type ConsentRepo interface {
	Create(ctx context.Context, c *Consent) error                        // text_version, given_at, channel, ip
	GetByContact(ctx context.Context, contactID string) (*Consent, error)
}

type ActivityLogRepo interface {
	Append(ctx context.Context, a *ActivityLog) error                    // audit: actor, action, entity, meta, at
	ListByEntity(ctx context.Context, entityType, entityID string) ([]*ActivityLog, error)
}

type UserRepo interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context) ([]*User, error)
}
```

> Typed-request repos (`SourcingRepo`, and later `ListingRepo`,
> `MarketEntryRequestRepo`) follow the same pattern. The demo's `SourcingRepo`
> (`Create`, `UpdateByLeadID`) moves verbatim. Adding a vertical = a new sibling
> repo, never a reshape of existing tables (`DATA_MODEL.md` rule 5).

### 3.8 `Clock` and `IDGen` — determinism seams

**Responsibility:** keep time and id generation out of the domain so use-cases are
deterministically testable.

```go
package domain

type Clock interface {
	Now() time.Time
}

type IDGen interface {
	NewID() string // UUID/ULID
}
```

**Production adapters:** a `time.Now` clock and a UUID/ULID generator (tiny
`internal/...` helpers). **Mocked in tests** with a frozen clock and a counting id
generator so assertions are stable.

---

## 4. The async agent-turn sequence (in detail)

This is the critical path (`ARCHITECTURE.md` §4, `REQUIREMENTS.md` FR-2.3/2.4). It
is split across two binaries: **`cmd/api`** (fast, synchronous ingest) and
**`cmd/worker`** (slow LLM/tool work). Nothing user-facing blocks on the LLM.

```
 Provider (WhatsApp / web widget)        cmd/api (Channel adapter)        Queue (Cloud Tasks)        cmd/worker (agent core)        Ports (LLM, repos, verify, GCS, mailer)
        │                                        │                                 │                              │                                  │
  1.    │ ── inbound message ───────────────────►│                                 │                              │                                  │
        │                                        │ 2. VerifyInbound(headers,body)  │                              │                                  │
        │                                        │    (X-Hub-Signature-256, etc.)  │                              │                                  │
        │                                        │    terminal-fail → 401, drop    │                              │                                  │
        │                                        │ 3. Normalize(body) → []Inbound  │                              │                                  │
        │                                        │ 4. dedupe by ProviderMsgID      │                              │                                  │
        │                                        │    (MessageRepo.SeenProviderMsg)│                              │                                  │
        │                                        │    already seen → ack, stop     │                              │                                  │
        │                                        │ 5. ensure Conversation exists   │                              │                                  │
        │                                        │    (GetByChannelRef|Create)     │                              │                                  │
        │                                        │ 6. Queue.Enqueue(job,           │                              │                                  │
        │                                        │      dedupeKey=chan:msgID) ────►│                              │                                  │
        │ ◄── 7. ACK FAST (HTTP 200) ────────────│                                 │                              │                                  │
        │     (WhatsApp) / WS "typing…" (widget) │                                 │ 8. push job ────────────────►│                                  │
        │                                        │                                 │                              │ 9. acquire per-conversation lock │
        │                                        │                                 │                              │    (pg advisory lock, §6)        │
        │                                        │                                 │                              │ 10. re-check dedupe inside lock  │
        │                                        │                                 │                              │     (SeenProviderMsg) idempotent │
        │                                        │                                 │                              │ 11. load state: Conversation +   │
        │                                        │                                 │                              │     Messages + Extracted ───────►│ (repos)
        │                                        │                                 │                              │ 12. flow engine: compute         │
        │                                        │                                 │                              │     missing required fields for  │
        │                                        │                                 │                              │     the vertical (Angrosist:     │
        │                                        │                                 │                              │     product, qty, location,      │
        │                                        │                                 │                              │     deadline, budget, CUI…)      │
        │                                        │                                 │                              │ 13. LLM.Complete(system+state+   │
        │                                        │                                 │                              │     tools) ─────────────────────►│ (LLM port)
        │                                        │                                 │                              │ ◄──── text and/or ToolCalls ─────│
        │                                        │                                 │                              │ 14. for each ToolCall:           │
        │                                        │                                 │                              │     execute against ports:       │
        │                                        │                                 │                              │       verify_company → Verify ──►│ (CompanyDataProvider + CompanyRepo)
        │                                        │                                 │                              │       upload_media   → Put ─────►│ (FileStore + DocumentRepo)
        │                                        │                                 │                              │       submit_lead    → Create ──►│ (Lead/Contact/typed-request repos)
        │                                        │                                 │                              │       handoff_to_human → flags  ►│ (LeadRepo.SetHumanFlags)
        │                                        │                                 │                              │     feed ToolResult back to LLM  │
        │                                        │                                 │                              │     (loop 13–14 until end_turn)  │
        │                                        │                                 │                              │ 15. persist: append messages,    │
        │                                        │                                 │                              │     UpdateExtracted, UpdateState,│
        │                                        │                                 │                              │     ActivityLog.Append (audit) ─►│ (repos)
        │                                        │                                 │                              │ 16. Channel.Send(reply) ────────►│ (Channel port)
        │ ◄────────── reply delivered ───────────┼─────────────────────────────────┼──────────────────────────────│                                  │
        │                                        │                                 │                              │ 17. ON COMPLETION (all required  │
        │                                        │                                 │                              │     fields + verified):          │
        │                                        │                                 │                              │     create normalized lead +     │
        │                                        │                                 │                              │     typed request, tag roles[],  │
        │                                        │                                 │                              │     Mailer.Send(confirmation +   │
        │                                        │                                 │                              │     staff notification) ────────►│ (Mailer)
        │                                        │                                 │                              │ 18. release lock; worker returns │
        │                                        │                                 │                              │     200 (success) or retry (§6)  │
```

Key properties:

- **Ack fast (steps 2–7).** WhatsApp gets HTTP 200 within its webhook budget; the
  widget gets an immediate WS/SSE acknowledgement / typing indicator. The LLM call
  happens only in the worker (step 13).
- **Idempotency is double-checked** — once at ingest (step 4) and again inside the
  lock (step 10) — because Cloud Tasks delivers at-least-once.
- **Ordering per conversation is preserved by the per-conversation lock** (step 9):
  two events for the same conversation never run concurrently; the second waits.
- **Tools run in the worker against ports** (step 14). The LLM only *requests*
  them. In the demo this logic lives in the Gemini runner; M1 moves it into
  `internal/agent` so it is LLM-vendor-agnostic.
- **Completion side effects** (step 17): the normalized `lead` + typed request,
  opportunistic `companies.roles[]` tagging, and staff notification. For handoff
  (`handoff_to_human`), set `needs_human=true`, `bot_active=false` and notify
  staff (FR-6).

Workers are stateless; scale by adding instances. Cloud Tasks provides durability +
retry/backoff.

---

## 5. THE SWAP RECIPE — how to add or replace an adapter

This is the payoff of hexagonal architecture and the part to get right. In every
case the rule is the same: **write a new type that satisfies the existing port,
then change one wiring line in the composition root.** The domain, the agent, the
flow engine, and every other adapter stay untouched.

General checklist (applies to all three cases):

1. **Find the port** in `internal/domain` (e.g. `Mailer`, `LLM`, `Channel`).
2. **Create a new adapter package** under the matching folder
   (`internal/email/<provider>`, `internal/agent/llm/<vendor>`,
   `internal/channels/<surface>`).
3. **Implement every method of the port.** Map the vendor's shapes to/from our
   neutral domain types. Read the vendor's URL/model/key **from `internal/config`**
   (Secret Manager) — never hardcode (§7).
4. **Write a contract test** that runs the same assertions the old adapter passed
   (the port is the contract). Keep the mock-based agent/use-case tests green.
5. **Flip the wiring** in `cmd/api` / `cmd/worker` (the composition root) — usually
   a single constructor swap, often selected by an env var.
6. **Update Terraform / Secret Manager** with any new secret or env, in
   `deploy/`. Do not commit secrets.

### (a) Swap the email provider (e.g. SendGrid → Brevo)

The agent calls `Mailer.Send(EmailMessage{Template, Locale, Data})`. It does not
know who delivers the mail.

1. Add `internal/email/brevo/brevo.go`:
   ```go
   package brevo

   type Mailer struct { apiKey, region string; templateIDs map[string]string }

   func New(cfg config.Email) *Mailer { /* read key/region from cfg */ }

   func (m *Mailer) Send(ctx context.Context, msg domain.EmailMessage) error {
       // map msg.Template + msg.Locale → Brevo template id;
       // map msg.Data → Brevo params; POST to Brevo; classify errors (§6).
   }
   ```
2. Contract test: feed a `lead_confirmation` (ro) + `staff_notification` (en),
   assert recipients/template mapping. (Run against Brevo sandbox or a recorded
   HTTP fixture.)
3. Wiring — in `cmd/api`/`cmd/worker` composition root, change:
   ```go
   // mailer := sendgrid.New(cfg.Email)
   mailer := brevo.New(cfg.Email)
   ```
   or make it config-driven: `mailer := email.New(cfg.Email)` where `email.New`
   switches on `cfg.Email.Provider` (`"sendgrid" | "brevo" | "mailgun"`).
4. Terraform: add the new provider's API key to Secret Manager; set
   `EMAIL_PROVIDER=brevo`, `EMAIL_REGION=eu`. Verify SPF/DKIM/DMARC for the new
   sender (FR-8.2). **Zero changes** to the agent, use-cases, or templates' call
   sites.

### (b) Swap the LLM provider (Claude ↔ Gemini)

Both are first-class: **Claude in production (Phase 1+), Gemini
(`gemini-2.0-flash-lite`) for the demo.** Because the agent depends only on the
`LLM` port (§3.2) and tools execute in `internal/agent` (not in the LLM adapter),
switching vendors is a wiring flip.

1. Both adapters already (M1) exist:
   - `internal/agent/llm/claude/claude.go` — maps `ToolSpec`/`ToolCall` to
     Anthropic tool-use; implements `Complete` + `Stream`.
   - `internal/agent/llm/gemini/gemini.go` — same against the Gemini SDK
     (`genai`). This is the demo's runner with the **tool-execution removed** —
     pure SDK glue.
2. Make selection config-driven in the composition root:
   ```go
   var llm domain.LLM
   switch cfg.LLM.Provider {           // "claude" | "gemini"
   case "gemini":
       llm = gemini.New(cfg.LLM)       // model from cfg.LLM.Model
   default:
       llm = claude.New(cfg.LLM)
   }
   agentCore := agent.New(llm, repos, verifier, fileStore, mailer, clock, idgen)
   ```
3. Config / Secret Manager:
   - demo: `LLM_PROVIDER=gemini`, `LLM_MODEL=gemini-2.0-flash-lite`,
     `GEMINI_API_KEY` (secret).
   - prod: `LLM_PROVIDER=claude`, `LLM_MODEL=claude-<id>`, `ANTHROPIC_API_KEY`
     (secret).
4. Tests: the agent/flow tests use the **scripted mock LLM** and are unaffected.
   Each adapter has its own tool-call-normalization contract test. **The system
   prompt, tool specs, flow engine, and tool implementations are shared** — only
   the wire format differs, and that difference is fully contained in the adapter.

> This is exactly why M1 splits the demo's Gemini runner: today tools live inside
> the Gemini code, so adopting Claude would mean re-implementing them. After the
> split, Claude is *purely additive*.

### (c) Add a new channel / interface for the AI agent (Telegram, Instagram, voice, …)

The agent is **channel-agnostic by design** (`CLAUDE.md` §3; FR-1.6). The same flow
engine, LLM port, tools, and lead pipeline power every surface. A new interface is a
new `Channel` adapter (§3.1) plus an inbound HTTP/WS route — **nothing in the agent
core changes.** This is the recipe the platform is built to make easy, because the
agent is meant to run on many interfaces.

Steps to add, e.g., **Telegram**:

1. **Create `internal/channels/telegram/telegram.go`** implementing
   `domain.Channel`:
   ```go
   package telegram

   type Channel struct { token, webhookSecret string; httpClient *http.Client }

   func New(cfg config.Telegram) *Channel { /* token/secret from cfg */ }

   func (c *Channel) Name() string { return "telegram" }

   // VerifyInbound: check Telegram's secret-token header on the webhook.
   func (c *Channel) VerifyInbound(h map[string]string, body []byte) error { ... }

   // Normalize: map a Telegram Update → []domain.InboundMessage
   // (ConversationRef = chat id, SenderRef = user id, ProviderMsgID = update/message id,
   //  MediaRefs from photo/document/voice).
   func (c *Channel) Normalize(body []byte) ([]domain.InboundMessage, error) { ... }

   // Send: call Telegram sendMessage / sendPhoto for OutboundMessage.
   func (c *Channel) Send(ctx context.Context, m domain.OutboundMessage) error { ... }

   // FetchMedia: getFile → download bytes (handed to FileStore for photos/voice).
   func (c *Channel) FetchMedia(ctx context.Context, ref domain.MediaRef) (io.ReadCloser, error) { ... }
   ```
2. **Add the inbound route** in `internal/api` / `cmd/api`:
   `POST /webhooks/telegram` → `VerifyInbound` → `Normalize` → dedupe → ensure
   conversation → `Queue.Enqueue` → **ack fast** (steps 2–7 of §4). This handler is
   a near-copy of the WhatsApp handler — only the channel adapter differs.
3. **Register the channel** so the worker can send replies on it. Keep a registry
   keyed by `Channel.Name()`:
   ```go
   channels := channel.Registry{
       "whatsapp":  whatsapp.New(cfg.WhatsApp),
       "webwidget": webwidget.New(cfg.Widget),
       "telegram":  telegram.New(cfg.Telegram),   // <-- the only new line in wiring
   }
   ```
   The worker resolves `channels[job.Channel]` to deliver the reply (step 16). The
   agent core, flow engine, tools, and lead creation are **identical** across
   channels.
4. **Channel-specific concerns stay in the adapter.** WhatsApp's 24h window +
   re-engagement templates live in the WhatsApp adapter; Telegram inline keyboards
   would live in the Telegram adapter; a **voice** channel's STT/TTS (speech↔text)
   lives entirely in a `internal/channels/voice` adapter that still produces/consumes
   `domain.InboundMessage`/`OutboundMessage` (it normalizes audio to text inbound and
   synthesizes audio from the agent's text outbound). The agent never knows the
   difference.
5. **Tests:** a fake `Channel` already exists for the flow tests; add a
   normalization contract test for Telegram payloads. **Config / Secret Manager:**
   `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET` as secrets; Terraform adds the
   webhook route. Mark WhatsApp's Meta-verification gating as the *only* channel
   that's gated on external business verification (`CLAUDE.md` invariant).

**Net effect:** adding a whole new platform for the agent touches exactly three
places — a new `internal/channels/<x>` package, one inbound route, and one registry
line — and zero lines of the agent, flow engine, tools, domain, or other adapters.

---

## 6. Error-handling model (queue worker)

The worker (`cmd/worker`, step 8+ of §4) must classify every failure so Cloud Tasks
retries the right ones and abandons the rest. The HTTP status the worker returns to
Cloud Tasks *is* the retry signal.

### 6.1 Retryable vs terminal

| Class | Examples | Worker returns | Cloud Tasks behavior |
|---|---|---|---|
| **Retryable (transient)** | LLM 429/5xx/timeout; DemoANAF "unavailable"; DB deadlock/connection blip; GCS/email 5xx; lock contention | `5xx` | retry with exponential backoff (configured `maxAttempts`, `minBackoff`/`maxBackoff`) |
| **Terminal (permanent)** | malformed/unparseable payload; unknown tool name; validation failure (e.g. required field semantically impossible); DemoANAF "not found"/"inactive" (a *business* outcome, not an error); auth/signature failure (already rejected at ingest) | `2xx` (ack — do not retry) | task completes; outcome is recorded in state/transcript, not retried |

Implementation: model this with sentinel/typed errors in the domain
(`domain.ErrRetryable`, `domain.ErrTerminal`, plus specific ones like
`ErrCompanyNotFound`, `ErrSourceUnavailable`). Adapters wrap vendor errors into
these (`fmt.Errorf("...: %w", err)`); the worker's top-level handler maps them to
HTTP status. **Business outcomes** (company not found/inactive) are *not* worker
errors — they become tool results fed back to the LLM (the demo already does this:
`verify_company` returns `{found:false, unavailable:…}` instead of failing the
turn). Distinguish `unavailable` (retryable) from `not found`/`inactive`
(terminal/business).

After `maxAttempts`, route the task to a **dead-letter** path (log to Cloud Logging
+ Sentry with the conversation correlation id; optionally flag the conversation
`needs_human`).

### 6.2 Idempotency

At-least-once delivery means a turn can be re-pushed. We dedupe **twice**:

- **At ingest** (§4 step 4) by `ProviderMsgID` via
  `MessageRepo.SeenProviderMsg`, and Cloud Tasks task-name dedupe
  (`dedupeKey = channel:providerMsgID`) so the same inbound enqueues once.
- **Inside the worker, under the lock** (§4 step 10) by the same key — the
  authoritative check. Side effects (lead creation, email, state advance) run only
  on the first successful pass; `MarkProviderMsgSeen` is committed in the same
  transaction as the state update so a retry after a partial failure is safe.

Each tool call should likewise be idempotent: `submit_lead` upserts (the demo's
`save_lead` already updates-in-place if a lead exists for the conversation);
`upload_media` keys GCS objects by `(owner, content-hash)`.

### 6.3 Per-conversation locking strategy

Goal: never process two turns of the same conversation concurrently (ordering +
no double-writes), without a global bottleneck.

- **Primary (Postgres advisory lock).** Since all durable state is already in Cloud
  SQL and Redis is optional early (`CLAUDE.md` §2), use
  `pg_advisory_xact_lock(hashtext(conversation_id))` taken at the start of the
  worker transaction (§4 step 9). It auto-releases at transaction end (commit or
  rollback) — crash-safe, no orphaned locks. A second concurrent turn for the same
  conversation blocks (or `pg_try_advisory_xact_lock` → return retryable `5xx` so
  Cloud Tasks re-pushes shortly after).
- **Alternative (Redis).** When Memorystore is introduced, a `SET lock:<convID> NX
  PX <ttl>` lease (with a watchdog/renew and TTL to survive crashes) is the
  equivalent. Use this only if advisory-lock contention becomes a measured problem;
  for the modest target load (NFR-5: single-digit concurrent chats), the Postgres
  advisory lock is sufficient and avoids a new dependency.

Either way the lock is **scoped to one conversation**; different conversations
process fully in parallel across worker instances.

---

## 7. Configuration & secrets (no hardcoding)

Every environment-specific value — service URLs, **model names**
(`claude-<id>` / `gemini-2.0-flash-lite`), bucket names, queue/region names, email
provider id, and **all keys/tokens** (DB creds, `ANTHROPIC_API_KEY`,
`GEMINI_API_KEY`, WhatsApp token + app secret + verify token, email API key,
channel webhook secrets) — is loaded by **`internal/config`** from **environment
variables**, with secrets sourced from **Secret Manager** at runtime. Nothing
secret is committed to the repo or baked into the image; nothing vendor-specific is
hardcoded in an adapter. This is what makes the swap recipes in §5 a config change
rather than a code change, and it satisfies `REQUIREMENTS.md` NFR-2/NFR-3 (secrets
only in Secret Manager, EU data residency). Three environments
(`development`/`staging`/`production`) each have their own GCP project/resources and
their own secrets; deploys happen from CI, never a laptop.
