---
description: Create a forward-only, additive PostgreSQL migration with its indexes and rollback notes, running the additive-only checklist.
argument-hint: <short description of the change>
---

Create a database migration for: `$ARGUMENTS`.

Steps:
1. Read `docs/specs/DATA_MODEL_DDL.md` (DDL source of truth), `backend/CLAUDE.md`, and the latest files in `backend/migrations/` to get the next sequence number and current shape.
2. Write a new forward-only, **additive** migration. Run the additive-only checklist:
   - ❏ No DROP/RENAME/reshape of populated `companies`, `contacts`, `leads`, `sourcing_requests`.
   - ❏ New verticals modeled as a **sibling typed-request table**, not a column on an existing one.
   - ❏ Extensible enums via lookup tables / documented TEXT sets — not native PG ENUM.
   - ❏ `companies` dedup stays UNIQUE `(country, reg_no)`; verification/financials nullable.
   - ❏ Correct ON DELETE behavior for the GDPR cascade (CASCADE personal graph, SET NULL company/assignee, RESTRICT taxonomy).
   - ❏ Files referenced via `documents.gcs_key`.
3. **Add every index the new query paths need in the same migration**, with a comment naming the query each serves. Use partial/GIN indexes where they pay.
4. Provide rollback notes (how to reverse, given forward-only policy).
5. Verify it applies on a clean database (via the migrate command / a disposable Postgres).

Report: tables/columns/indexes added, the query patterns served, invariants checked, and apply status. Consider delegating to the `db-migration-engineer` agent for complex schema work.
