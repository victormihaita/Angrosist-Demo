---
description: Scaffold a new adapter behind an existing port (the modular-swap workflow) with its mock and contract test.
argument-hint: <port-name> <adapter-name>  e.g. Mailer brevo
---

Create a new adapter that implements an existing port — the core modular-swap workflow. Arguments: `$ARGUMENTS` (port name, then adapter name).

Steps:
1. Read `docs/specs/ARCHITECTURE_DETAIL.md` and locate the named **port interface** (do not modify it — adapters conform to ports, never the reverse).
2. Read an existing adapter for the same port (if any) as a template, plus `backend/CLAUDE.md`.
3. Create the adapter package under `internal/<area>/<adapter>/` implementing every method of the port. All config (URLs, keys, model names) comes from `internal/config` via env/Secret Manager — nothing hardcoded.
4. Wire it in `cmd/*` (dependency injection) behind a config flag so the provider is selectable by env.
5. Add a **mock/fake** for the port (or extend the shared mock) and a **contract test** that runs the same assertions against the new adapter as the existing one.
6. Apply error handling: timeouts, retry/backoff, retryable-vs-terminal classification.
7. Run `gofmt`, `go build ./...`, `go test ./...`.

Report: the port, the new adapter, how to select it via env, and test status. Confirm the port interface was not changed and no secret/URL was hardcoded.
