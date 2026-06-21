# Documentation Index — Euro Intermed B2B Platform

Specification set for Claude Code. Read in this order.

1. **CLAUDE.md** — rules, conventions, stack, and invariants. The entry point Claude Code reads first.
2. **PRODUCT.md** — what we're building, for whom, the verticals/channels, and scope boundaries.
3. **REQUIREMENTS.md** — functional (FR) + non-functional (NFR) requirements, phase-tagged, plus explicit out-of-scope.
4. **ARCHITECTURE.md** — clean (hexagonal) architecture, the async agent runtime, the GCP deployment topology, and the Docker backend.
5. **DATA_MODEL.md** — entities, enums, Phase-2 additions, and the invariants that keep the model extensible.
6. **DEVELOPMENT_PLAN.md** — Milestone 0 (demo) → Phase 1 → Gate → Phase 2, each with deliverables and acceptance criteria.

## Fixed decisions (single source of truth)

- **Everything runs on Google Cloud Platform.** Backend ships as a **Docker image**; DB is **Cloud SQL (PostgreSQL)**; secrets in **Secret Manager**; files in **GCS**; queue **Cloud Tasks**; cron **Cloud Scheduler**; images in **Artifact Registry**. Frontend on GCP (Firebase Hosting / Cloud Run).
- **Go** backend, **React/TypeScript** frontend, **Claude** for the LLM, **WhatsApp Cloud API**, **DemoANAF** for verification, an external EU email provider.
- **Channel-agnostic agent**, **hexagonal architecture**, **async agent turns**, **stateless services**, **EU data residency**.
- Phase 1 (Angrosist + PalletClearance) €5.000 · Phase 2 (SkalYou Core MVP) €5.000 · Phase 3 (first Country Operator) €2.000 — all on milestones, **paid on delivery (no advance)**. Maintenance **€280/lună** (7h incl., €40/h extra) + monthly roadmap meeting. Performance bonuses (€500 first enterprise client, €500 monthly-revenue threshold, annual KPI bonus). No equity at this stage. A 1–2 week trial/demo module first (Milestone 0).
