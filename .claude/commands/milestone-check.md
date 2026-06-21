---
description: Verify a milestone's acceptance criteria from BUILD_PLAN.md against the current codebase and report what's done vs outstanding.
argument-hint: <milestone>  e.g. M1, M3, M2.1
---

Verify milestone `$ARGUMENTS` against `docs/BUILD_PLAN.md`.

Steps:
1. Read the milestone's epics, tasks, acceptance criteria, and "definition of done" in `docs/BUILD_PLAN.md`, plus the cross-cutting DoD at the bottom.
2. For each task/criterion, check the codebase for evidence it's satisfied (files, migrations, tests, config, endpoints in `openapi.yaml`). Run available checks: `go build ./...`, `go test ./...`, `npm run lint`/`build`, openapi validate, and `/secret-scan`.
3. Apply the cross-cutting gates: no hardcoded secrets/URLs; new I/O behind ports with mocks; migrations additive + indexed; input validation; endpoints documented; critical-path tests green; security/GDPR review where personal data is touched.
4. For the milestone's specific acceptance criteria, state PASS/FAIL/PARTIAL with the evidence (or the gap).

Report a checklist: criterion → status → evidence/gap. End with a verdict (milestone done? what's blocking?) and recommend next actions. For security-sensitive milestones (M5), suggest running the `security-gdpr-auditor` agent.
