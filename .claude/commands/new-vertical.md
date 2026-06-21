---
description: Add a new vertical (sibling typed-request table + flow definition + prompts) without touching existing data — encodes the "lead is a thin event" invariant.
argument-hint: <vertical-name> <intent: buy|sell|...>
---

Add a new vertical end-to-end: `$ARGUMENTS`. A vertical is additive by design — a lead is a thin event pointing at one typed request, so a new vertical = a new sibling table + flow + prompts, never a reshape.

Steps:
1. Read `docs/DATA_MODEL.md` + `docs/specs/DATA_MODEL_DDL.md` (thin-lead invariant), `docs/specs/AI_AGENT_SPEC.md` (flow engine + prompt library), `docs/REQUIREMENTS.md` (FR-3 vertical flows).
2. **Data:** register the new value in the `vertical` (and `intent` if needed) lookup table. Create a new **sibling typed-request table** for this vertical's fields (like `sourcing_requests`/`listings`). Add indexes. Use `/new-migration`. Do not alter existing typed-request tables.
3. **Flow engine:** add the deterministic required-field definition for the vertical (and any blocking rule, e.g. mandatory photos block `submit_lead`).
4. **Prompts:** add versioned RO + EN system prompts for the vertical to the prompt library (externalized, swappable). Add tool usage notes if it needs a new/changed tool.
5. **Lead creation:** extend `submit_lead` so it writes the lead + the new typed request transactionally.
6. **Dashboard:** ensure the new vertical appears in pipeline filters and lead detail renders its typed request (shadcn, per UI/UX guide).
7. **Evals:** add eval scenarios for the new flow (`/agent-eval`).
8. Tests + lint + build.

Report: the new table, flow definition, prompts (RO/EN), tool changes, and confirmation that no existing vertical's data/tables were reshaped.
