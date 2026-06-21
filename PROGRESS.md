# PROGRESS.md — Where we are right now

> **This is the file to open to see what's done, what's happening, and what's next.** Updated and committed after every unit of work. The full task list lives in `docs/BUILD_PLAN.md`; this is the running status on top of it.

**Last updated:** 2026-06-21
**Current milestone:** M0.5 — Demo hardening
**Current task:** _starting H1 (secrets & config hygiene)_
**Branch:** `main` · **Origin:** in sync after each push

---

## Execution order (the plan I'm following)

1. ✅ **Setup** — Claude OS, hard rules, BUILD_PLAN, specs, MCP, hooks _(done & merged)_
2. ✅ **User journeys doc** — confirm the final outcome _(done)_
3. ⏳ **M0.5 — Demo hardening** — close audit gaps on the existing demo _(in progress)_
4. ⬜ **M1 — Foundation** — refactor to `/cmd`+`/internal`, GCP/Terraform, CI/CD, full schema
5. ⬜ **M2 — Agent core + web widget** — channel-agnostic agent, async runtime, Claude LLM, WS/SSE
6. ⬜ **M3 — Angrosist + dashboard** — Angrosist LIVE end-to-end
7. ⬜ **M4 — WhatsApp + PalletClearance + photos + i18n**
8. ⬜ **M5 — KPIs, GDPR/security, backup, testing, handover**
9. ⬜ **Phase 2 (M2.1–M2.3)** — SkalYou marketplace
10. ⬜ **Phase 3** — Country Operator

Legend: ✅ done · ⏳ in progress · ⬜ not started

---

## M0.5 — Demo hardening (live checklist)

> Goal: make the demo safe and modular before scaling it into Phase 1. Source: `docs/BUILD_PLAN.md` → M0.5.

### H1 — Secrets & config hygiene
- [ ] Confirm `.env` gitignored; document the test-keys decision (kept as-is per owner)
- [ ] Move Gemini model name + DemoANAF base URL fully to env (no source defaults)
- [ ] Audit both `.env.example` files for completeness

### H2 — API surface safety
- [ ] Lock CORS to an env-configured allowlist (replace `*`)
- [ ] Input validation on `/api/chat` (message length, conversation_id) + CUI format
- [ ] Request size limit + basic rate limiting

### H3 — LLM behind a port
- [ ] Introduce provider-neutral `LLM` port; move Gemini code into an adapter
- [ ] Retry/backoff + timeout + graceful failure on LLM & ANAF calls

### H4 — Baseline tests & errors
- [ ] Unit-test qualification/extraction use-case with a mock LLM port
- [ ] Extend ANAF adapter contract tests
- [ ] Frontend error boundary + visible network-error toast

---

## Changelog (newest first)

- **2026-06-21** — Added `docs/USER_JOURNEYS.md` (all actor journeys) + this tracker. Starting M0.5.
- **2026-06-21** — Merged Claude OS + planning suite to `main` (hard rules, BUILD_PLAN, 6 specs, 7 agents, 6 commands, MCP, hooks).
- **2026-06-21** — Wired Context7 + shadcn MCP (key in git-ignored local settings).
