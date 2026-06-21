---
description: Scan the repo (or staged changes) for hardcoded secrets and URLs before commit; report any violations of the env-only rule.
argument-hint: [--staged | --all]   default: --staged
allowed-tools: Bash(git diff:*), Bash(git status:*), Bash(grep:*), Bash(rg:*), Grep
---

Scan for hardcoded secrets and URLs that violate Hard Rule #1 (env-only). Scope: `${ARGUMENTS:---staged}`.

Steps:
1. Choose the target: `--staged` → `git diff --cached`; `--all` → the whole tree (excluding `.env` files, `node_modules`, `dist*`, `vendor`, build artifacts).
2. Search for high-signal patterns:
   - API-key shapes: `AIza[0-9A-Za-z_\-]{20,}` (Google/Gemini), `sk-ant-[A-Za-z0-9\-]+` (Anthropic), `sk-[A-Za-z0-9]{20,}`, AWS `AKIA[0-9A-Z]{16}`, bearer/JWT-looking blobs.
   - Postgres/Neon URLs: `postgres(ql)?://[^ \n]+:[^ @\n]+@`.
   - Generic assignments: `(?i)(api[_-]?key|secret|token|password|passwd|client[_-]?secret)\s*[:=]\s*["'][^"']+["']` with a non-placeholder value.
   - Hardcoded http(s):// literals in source (allow them only in docs, tests, and `.example` files; flag in `.go`/`.ts`/`.tsx`).
3. Treat `.env.example` placeholders (no real value) as OK. Treat real values in tracked files as violations.
4. Confirm `backend/.env` and `frontend/.env` are gitignored and not staged.

Report each hit as file:line + matched pattern + severity, with the remediation (move to env/Secret Manager via `internal/config` or `VITE_*`). If clean, state that explicitly and list what was scanned. This is the same check the pre-commit hook runs — never bypass it.
