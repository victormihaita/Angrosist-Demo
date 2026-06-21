# Contributing

> How we work on the Euro Intermed B2B platform. Read `CLAUDE.md` (Hard rules) first; this doc covers process.

## Golden rules (the short list)

1. **No hardcoded secrets or URLs.** Env / Secret Manager only. The pre-commit hook will block you otherwise.
2. **Ports & adapters for all I/O.** Domain imports no infrastructure. Swap a provider = new adapter.
3. **The agent is channel-agnostic.** Add a channel/interface without touching the agent core.
4. **Migrations are additive and forward-only.** Never reshape existing company/lead data.
5. **Validate every external input. Document every endpoint. Tests gate "done".**

Full list: `CLAUDE.md` → *Hard rules (non-negotiable)*.

## Project setup

### Backend (`backend/`, Go)
```bash
cp .env.example .env          # fill values — DATABASE_URL, LLM key, DEMOANAF_BASE_URL, ...
go run ./cmd/migrate          # apply migrations
go run ./cmd/server           # API
go test ./...                 # tests
gofmt -l . && go vet ./...    # format + vet
golangci-lint run             # lint (config: .golangci.yml)
```

### Frontend (`frontend/`, React/TS)
```bash
cp .env.example .env          # set VITE_API_URL
npm install
npm run dev                   # app
npm run build && npm run build:widget
npm run lint                  # eslint
npx prettier --write .        # format (config: .prettierrc.json)
```

The backend and frontend are **independently deployable**. They communicate only over the documented API (`docs/specs/openapi.yaml`).

## Branching, commits, PRs

- Branch off `main`: `feat/...`, `fix/...`, `chore/...`, `docs/...`.
- Small, focused commits with imperative messages (`add WhatsApp signature verification`).
- A PR should map to a task/epic in `docs/BUILD_PLAN.md`. In the description, link the milestone/task and note which acceptance criteria it advances.
- **PR checklist** (also the cross-cutting DoD in BUILD_PLAN):
  - [ ] No hardcoded secrets/URLs (`/secret-scan` clean).
  - [ ] New I/O behind a port with a mock; dependency rule holds.
  - [ ] Migrations additive; indexes added for new query paths.
  - [ ] Input validation on new external inputs; parameterized SQL.
  - [ ] New endpoints reflected in `openapi.yaml`; new packages documented.
  - [ ] Critical-path tests added/updated and green.
  - [ ] Security/GDPR review (`security-gdpr-auditor`) if it touches personal data, auth, or config.

## The modular-swap recipe (core to this codebase)

To replace or add a provider behind an existing port (email, LLM, channel, storage, …):

1. Find the port interface in `docs/specs/ARCHITECTURE_DETAIL.md` — **do not change it**.
2. Run `/new-adapter <Port> <name>` to scaffold the adapter + mock + contract test.
3. Read config (URL, key, model name) from `internal/config` (env/Secret Manager) — never inline.
4. Select the adapter via an env flag in `cmd/*` wiring.
5. The contract test runs the same assertions against the new adapter as the old one.

Adding a **new agent channel/interface** (Telegram, Instagram, voice, …): implement the `Channel` port in `internal/channels/<x>/`, add one route + one registry line. The agent core is untouched. See the "add a channel" walkthrough in `ARCHITECTURE_DETAIL.md`.

Adding a **new vertical**: run `/new-vertical` — it creates a sibling typed-request table + flow definition + RO/EN prompts without reshaping existing data.

## Testing expectations

- **Unit:** domain use-cases + flow engine, mock ports.
- **Integration:** repositories against a real Postgres.
- **Contract:** each external adapter.
- Critical paths (qualification, verification, lead creation, handoff, cascade erasure) covered before a milestone is "done".

## Claude tooling

- **Agents** (`.claude/agents/`): delegate specialized work — `go-backend-architect`, `db-migration-engineer`, `agent-prompt-engineer`, `frontend-shadcn-builder`, `security-gdpr-auditor`, `infra-terraform-engineer`, `api-contract-keeper`.
- **Commands** (`.claude/commands/`): `/new-adapter`, `/new-migration`, `/new-vertical`, `/agent-eval`, `/secret-scan`, `/milestone-check`.
- **MCP** (`.mcp.json`): shadcn/ui (UI components) + Context7 (library docs). Set `CONTEXT7_API_KEY` in your environment. The shadcn MCP reads `frontend/components.json`; run Claude from the repo root and target the frontend when adding components.
- **Hooks** (`.claude/settings.json`): a pre-commit secret/URL scan (blocks bad commits) and format-on-save (gofmt/prettier).

## Deployment

- Deploys happen from **CI, never a laptop**. GitHub Actions builds the Docker image → Artifact Registry → Cloud Run; migrations run as a deploy step.
- Three environments: dev / staging / prod, each with its own GCP project/resources and secrets. Production is GCP (EU regions); the M0 demo uses Vercel + Neon for speed.
