# ANAF_API.md — Company verification provider audit & decision

> Audit of **DemoANAF.ro** (`https://demoanaf.ro/api`) as our company-verification data source, what tiers exist, exactly which endpoints/fields we need, and how it maps to our data model. Sits behind the `CompanyDataProvider` port (`docs/specs/ARCHITECTURE_DETAIL.md`) so the provider is swappable.
>
> **Audited:** 2026-06-21 (live API + `https://demoanaf.ro/api-docs`).

---

## TL;DR decision

- **Use DemoANAF.ro free REST API** as the Phase-1 verification provider. It's free, keyless, and **far richer than the raw ANAF web service** the demo currently calls (it adds ONRC registration, administrators, full CAEN list, financials, name search).
- **Endpoints we need:** `/company/:cui` (core), `/company/:cui/financials` (optional enrichment), `/caen` (CAEN→roles/categories), `/search` (name fallback when no CUI). Skip `/rates`, `/iban`, `/press` for now (`/rates` may return for P2 multi-currency).
- **Tier:** Free (300 req/min) is sufficient for our load (NFR-5: single- to low-double-digit concurrent chats). **Pro/MCP** is only needed at higher volume or for an SLA.
- **⚠️ Two risks to resolve before client handover:** (1) the demo adapter currently points at the *wrong* endpoint (raw ANAF, not DemoANAF) — reconcile in M1; (2) DemoANAF publishes **no commercial-use terms or uptime SLA** — confirm licensing / arrange a fallback. The `CompanyDataProvider` port makes swapping providers a zero-domain-change task, which de-risks this.

---

## Provider facts

| Property | Value |
|---|---|
| Base URL | `https://demoanaf.ro/api` |
| Auth | **None** — no account, no API key ("fără cont, fără cheie API") |
| Rate limit (free) | **300 requests / minute** |
| Response envelope | `{ "success": bool, "data": {...}, "meta": {...} }` |
| Data sources | Public ANAF + ONRC + BNR, cached |
| Freshness | Per-endpoint cache, minutes → days; `meta.cached` + `meta.cachedAt` exposed |
| Tiers | **Free** (public REST) · **Pro** (extended tools via MCP connector — volume; pricing/terms not published) |
| MCP | A Pro MCP connector is advertised for "large volumes and extended tools" — details not public; **for AI assistants, not our runtime** (see note below) |
| Terms of service | **Not published** — commercial-use rights, SLA, and retention are undocumented (risk) |

### Endpoints

| Method | Path | Purpose | We use it? |
|---|---|---|---|
| GET | `/company/:cui` | Company identity, VAT, ONRC status, administrators, CAEN | ✅ **core** |
| GET | `/company/:cui/financials` | ~8 years of financials (turnover, profit, employees…) | ✅ optional enrichment |
| GET | `/search?q=` | Name search across 4M+ ONRC records | ✅ fallback (no CUI) |
| GET | `/caen?q=` | CAEN Rev.2 activity-code search | ✅ map CAEN→roles/categories |
| GET | `/rates` | BNR exchange rates | ⬜ later (P2 multi-currency) |
| GET | `/iban` | Treasury IBAN lookup | ⬜ not needed |
| GET | `/press` | ANAF press releases | ⬜ not needed |

---

## `/company/:cui` — the field map (what we actually persist)

Example: `GET https://demoanaf.ro/api/company/14399840` → DANTE INTERNATIONAL SA.

**Identity:** `cui`, `name`, `registrationNumber` (ONRC J-number), `registrationDate`, `legalForm` (SA/SRL…), `ownershipForm`, `organizationForm`.
**Address:** `address`, `postalCode`, `headquartersAddress{street,number,locality,county,country,postalCode}`.
**VAT/tax:** `vatRegistered`, `vatStatus`, `vatPeriods[]`, `vatCheckedAt`, `splitVat`, `cashBasisVat`, `datoriiAnaf`, `fiscalAuthority`.
**Registration status:** `registrationState`, `inactive`, `onrcStatus`, `onrcStatusLabel` (e.g. "Funcțiune").
**Administrators:** `administrators[]{name, role, personId}`.
**Activity:** `caenCode` (primary), `authorizedCaenCodes[]` (all permitted codes).
**E-invoicing:** `eFacturaRegistered`.
**Meta:** `{ cached, source, cachedAt }`.

### Mapping to our data model (`docs/specs/DATA_MODEL_DDL.md`)

| Our column | Source field |
|---|---|
| `companies.country` | constant `RO` (this provider) |
| `companies.reg_no` | `cui` (dedup key is `(country, reg_no)`) |
| `companies.name` | `name` (title-cased) |
| `companies.vat_status` | `vatStatus` / `vatRegistered` |
| `companies.caen` | `caenCode` |
| `companies.address` | `headquartersAddress` (structured) |
| `companies.roles[]` | **derived** from `caenCode` + `authorizedCaenCodes[]` (CAEN→role mapping; opportunistic) |
| `company_verifications.source` | `"demoanaf"` |
| `company_verifications.vat_status` | `vatStatus` |
| `company_verifications.administrators` | `administrators[]` |
| `company_verifications.raw` | full `data` JSON (jsonb) |
| `company_verifications.checked_at` | now / `meta.cachedAt` |

> The richer fields (administrators, ONRC status, e-Factura, authorized CAEN list) let us auto-tag `roles[]` and give consultants real context — a big upgrade over the raw-ANAF VAT-only response.

## `/company/:cui/financials` — the field map (optional, valuable)

Returns an array of annual records (~7–8 years). Per year: `year`, `cui`, `companyName`, `caenCode`, `caenDescription`, `eurRate`, and an `indicators[]` set including **net turnover (I13)**, total revenue (I14), total expenses (I15), gross/net profit (I16–I19), assets (I1–I5), liabilities/equity (I7,I10,I11), and **average employees (I20)**. `meta` reports completeness (`financialsExpectedYears`, `financialsCoveredYears`, `financialsMissingYears`).

→ Persist into `company_financials` (`year`, `turnover`, `raw` jsonb). **Use:** lead scoring / prioritization (turnover & employee count signal buyer size), shown in the dashboard company panel. Optional/nullable per the data model.

---

## ⚠️ Discrepancy to fix: the demo calls the wrong API

The current demo adapter (`backend/pkg/adapters/anaf/client.go`) calls the **raw ANAF web service** `https://webservicesp.anaf.ro/api/PlatitorTvaRest/v9/tva` (VAT-payer registry), via the `DEMOANAF_BASE_URL` env var — despite the name. That endpoint only returns VAT status + basic name/address, **not** ONRC registration, administrators, CAEN list, or financials.

**Reconciliation (M1, behind the `CompanyDataProvider` port):**
- Point the adapter at `https://demoanaf.ro/api/company/:cui` and map the richer response (table above).
- Add a financials call (`/company/:cui/financials`) gated by a config flag.
- Keep the raw-ANAF client available as a **second adapter / fallback** (same port) for resilience — useful given DemoANAF has no SLA.
- All base URLs stay in env (`ANAF_*_BASE_URL`), never hardcoded.

---

## Free vs Pro — what we need

| Need | Free tier | Verdict |
|---|---|---|
| CUI verification in conversation | ✅ keyless, 300 req/min | **Free is enough for MVP** |
| Financial enrichment | ✅ included | Free |
| Name search / CAEN | ✅ included | Free |
| High volume / burst > 300 rpm | ❌ | Pro (MCP) — only if we scale past it |
| Uptime SLA / commercial guarantee | ❌ not published | **Action: confirm with provider before handover** |

**Recommendation:** ship on **Free**. Our concurrency target is low; 300 req/min ≫ our need, and we cache verifications in `company_verifications` so repeat CUIs don't re-hit the API. Revisit Pro only if volume or an SLA requirement appears.

### About their MCP connector

DemoANAF's "Pro via MCP connector" is an MCP server aimed at **AI assistants/agents consuming the data interactively** — not something our Go backend calls at runtime. Our architecture is: the LLM emits a `verify_company` tool call → our code → `CompanyDataProvider` port → **REST adapter**. So we consume the **REST API**, not their MCP, in production. (Their MCP could optionally be added to a developer's Claude setup for ad-hoc lookups, but it's not part of the platform runtime.)

---

## Action items (tracked in BUILD_PLAN)

- [ ] **M1:** repoint the verification adapter to `demoanaf.ro/api/company/:cui`; map the rich response into `companies` + `company_verifications`; derive `roles[]` from CAEN. _(db-migration-engineer + go-backend-architect)_
- [ ] **M1:** add optional `/financials` enrichment behind a config flag → `company_financials`.
- [ ] **M1:** keep a raw-ANAF fallback adapter behind the same port for resilience.
- [ ] **Pre-handover:** confirm DemoANAF commercial-use terms / SLA; decide Free vs Pro; document the fallback provider.
- [ ] **Build a CAEN→roles/categories mapping table** (seeds the B2B directory + Phase-2 matching).
