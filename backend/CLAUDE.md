# backend/CLAUDE.md — Go service conventions

> Layer-specific rules for the Go backend. Inherits all **Hard rules** from the root `CLAUDE.md`.
> Module: `github.com/angrosist/demo`. Production runtime: Docker image on Cloud Run.

## Target layout (refactor into this during M1)

```
backend/
  cmd/                  # entrypoints — thin wiring only
    server/             # HTTP API + widget WS/SSE
    worker/             # Cloud Tasks worker endpoint (agent turns)
    migrate/            # migration runner
  internal/
    domain/             # entities + use-cases. NO framework/infra imports.
    agent/              # channel-agnostic flow engine, LLM port, tools, prompt loading
    channels/           # adapters: webwidget/, whatsapp/ (+ future: telegram/, voice/)
    verification/       # DemoANAF adapter (port: CompanyDataProvider)
    persistence/        # Postgres repositories (port: Repositories)
    storage/            # GCS adapter (port: FileStore)
    email/              # email provider adapter (port: Mailer)
    queue/              # Cloud Tasks adapter (port: Queue)
    api/                # HTTP handlers: dashboard API, webhook, widget transport
    config/             # env + Secret Manager loading
  migrations/           # SQL migrations (forward-only, additive)
```

> The demo currently lives under `backend/pkg/{domain,usecases,adapters,ports}` and `backend/api/`. M1 migrates it into the layout above. Keep behavior; move packages; do not rewrite the data model.

## The dependency rule

`internal/domain` and the agent flow engine import **nothing** from `channels`, `persistence`, `storage`, `email`, `queue`, `verification`, `api`, or any vendor SDK. Dependencies point inward only. Ports are declared where they're consumed (domain/agent); adapters implement them in their own packages and are wired in `cmd/*`.

## Ports & adapters

- Every external call (LLM, DemoANAF, WhatsApp, GCS, email, queue, DB) goes through a port interface (see `docs/specs/ARCHITECTURE_DETAIL.md`). One production adapter per port; a mock/fake for tests.
- To swap or add a provider, write a new adapter satisfying the existing port — never change the port to fit a vendor. Run `/new-adapter`.
- The LLM port is provider-neutral: Claude (prod) and Gemini (demo) are two adapters behind it. Model name, temperature, max tokens come from config, never hardcoded.

## Errors

- Wrap with context: `fmt.Errorf("verify company %s: %w", cui, err)`. Never swallow errors.
- Classify for the worker: **retryable** (timeouts, 5xx, transient DB) → return so Cloud Tasks retries with backoff; **terminal** (4xx, validation, bad input) → ack and do not retry. Surface this distinction explicitly (typed/sentinel errors).
- External-call adapters apply retry/backoff and timeouts; never block a request thread on an unbounded call.

## Async agent runtime

- API/webhook handlers **ack fast** and enqueue; the `worker` runs the LLM/tool turn.
- **Idempotency:** dedupe inbound by provider message ID before enqueue.
- **Per-conversation lock:** Postgres advisory lock (or Redis) so two events for one conversation never process concurrently; preserves message ordering.
- Workers are stateless and disposable; all state in Postgres/GCS.

## Migrations

- Forward-only, additive. Never drop/rename/reshape existing `companies`, `contacts`, `leads`, `sourcing_requests`.
- One concern per migration; include the index in the same migration that needs it. Use `/new-migration`.
- Migrations run as part of deploy, not by hand. The DDL source of truth is `docs/specs/DATA_MODEL_DDL.md`.

## Config & secrets

- All config via env; all secrets via Secret Manager (prod) / gitignored `.env` (local). `internal/config` is the only place that reads the environment.
- Nothing secret in source, logs, or errors. The pre-commit secret scan is mandatory.

## Logging & observability

- Structured JSON logs with a request/conversation correlation ID propagated through the turn. No PII beyond what's necessary (log `contact_id`, not the phone number).
- Wire Cloud Logging + Cloud Monitoring + Sentry (M5). Emit metrics on the hot paths.

## Testing

- **Unit:** domain use-cases and the flow engine (table-driven, mock ports).
- **Integration:** repositories against a real Postgres (testcontainers or a disposable DB).
- **Contract:** each external adapter against a recorded/faked vendor.
- Critical paths covered before a milestone is "done": qualification, verification, lead creation, handoff, cascade erasure.
- `gofmt`/`goimports` clean; `golangci-lint` green (config at repo root).

## Security (see docs/specs/SECURITY.md)

- Parameterized SQL only. Validate every external input at the adapter/handler boundary.
- Verify WhatsApp `X-Hub-Signature-256` on every webhook; reject + log on failure.
- CORS locked to known origins (no `*` in prod). RBAC enforced in the dashboard API. Least-privilege service accounts.
