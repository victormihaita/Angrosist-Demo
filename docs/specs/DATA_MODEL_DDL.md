# DATA_MODEL_DDL.md — PostgreSQL DDL Specification (handover-grade)

> **Status:** Reference specification. The DDL below is the **target schema** for the Euro Intermed B2B
> platform. It is the canonical source for writing forward-only migrations under `backend/migrations/`.
> **This document does not create migration files** — it is the engineering reference the migrations
> implement against.
>
> **Source of truth:** `docs/DATA_MODEL.md`. This file expands that model into production-grade
> PostgreSQL DDL with keys, constraints, indexes, GDPR cascade design, and query-pattern notes.
>
> **Conventions:** PostgreSQL 15+, EU residency (Cloud SQL `europe-*`). All timestamps are
> `TIMESTAMPTZ` (UTC). All primary keys are `UUID DEFAULT gen_random_uuid()` (requires the built-in
> `pgcrypto` / `gen_random_uuid()`; on PG13+ it is available via `pgcrypto`). Money is `NUMERIC(14,2)`,
> never `float`. Text is `TEXT` (no arbitrary `VARCHAR(n)` limits). Snake_case everywhere.

---

## 0. What the demo (Milestone 0) already created — reconciliation note

The live demo migrations (`backend/migrations/001`–`008`) created a **thinner, demo-shaped** subset:

| Demo table | Demo shape | Target shape (this spec) | Migration path (forward-only) |
|---|---|---|---|
| `companies` | `cui TEXT UNIQUE`, `name`, `address`, `county`, `is_active`, `raw_data`, `verified_at` | `country` + `reg_no` dedup, `roles[]`, `vat_status`, `caen` | **Additive**: add new columns nullable; backfill `country='RO'`, `reg_no = cui`; add the `(country, reg_no)` unique index; keep `cui` as a generated/kept alias column until callers migrate. **Do not drop `cui`.** |
| `conversations` | `channel`, `state`, `extracted` jsonb | unchanged + add `contact_id`, `lead_id`, `language`, `bot_active`, `dedup_key` | Additive columns only |
| `messages` | `conversation_id`, `role`, `content`, `tool_calls`, `tool_call_id` | unchanged | Already correct |
| `contacts` | `company_id`, `name`, `phone`, `email` | + `language`, `consent_id` | Additive columns only |
| `leads` | `conversation_id` (UNIQUE), `company_id`, `contact_id`, `status` | + `assigned_to`, `vertical`, `intent`, `source`, `summary`, `needs_human`, `bot_active`, `updated_at` | Additive columns only; keep `conversation_id` + its unique constraint |
| `sourcing_requests` | `lead_id`, `product_name`, `quantity`, `unit`, `delivery_location` | + `category_id`, `recurring`, `deadline`, `budget` | Additive columns only |

**Rule (invariant #8):** migrations are **forward-only and additive**. We never reshape, drop, or rename
existing `companies` / `contacts` / `leads` / `sourcing_requests` columns that hold demo data. New shape is
reached by *adding* columns/tables and backfilling, never by `DROP`/`RENAME` on populated fields. Where the
demo name differs (`product_name` vs spec `product`, `cui` vs `reg_no`), we **keep the demo column** and add
the canonical one alongside; the repository layer reads the canonical one.

---

## 1. Relations overview (ASCII ER diagram)

Cardinalities: `1` = exactly one, `0..1` = optional one, `*` = many. `→` points to the FK target (the "one").

```
                         ┌──────────────┐         ┌─────────────────────┐
                         │  categories  │◄────────│  (self parent_id)   │  hierarchical taxonomy
                         │  (self-FK)   │          └─────────────────────┘
                         └──────┬───────┘
                                │ 1
            ┌───────────────────┼─────────────────────────────────────────┐
            │ *                 │ *                  │ *                    │ *
   ┌────────┴────────┐ ┌────────┴────────┐ ┌─────────┴──────────┐ ┌────────┴─────────────┐
   │ sourcing_       │ │   listings      │ │ market_entry_      │ │  buyer_profiles      │
   │ requests        │ │  [Palletclr]    │ │ requests [P2]      │ │  (standing demand)   │
   │ (Angrosist)     │ │                 │ │ (SkalYou)          │ │                      │
   └───────┬─────────┘ └───────┬─────────┘ └─────────┬──────────┘ └────────┬─────────────┘
           │ 1               1 │                    1 │                    * │
           │  (typed request, exactly one per lead)   │                      │
           │                   │                      │                      │
        ┌──┴───────────────────┴──────────────────────┴──┐                   │
        │                    leads                        │                   │
        │  (THIN EVENT: vertical, intent, status,         │                   │
        │   needs_human, bot_active, assigned_to)         │                   │
        └──┬───────────────┬───────────────┬─────────────┘                   │
           │ *             │ *             │ 0..1                             │
           │               │               │ assigned_to                      │
   ┌───────┴──────┐ ┌──────┴───────┐ ┌─────┴──────┐                           │
   │   contacts   │ │  companies   │ │   users    │                           │
   │              │ │ (B2B asset)  │◄┘ (staff/admin)                          │
   │ consent_id ──┼─┐  roles[]     │◄──────────────────────────────────────┘ │
   └──────┬───────┘ │ UNIQUE(      │                                          (company_id)
          │ 1       │  country,    │
          │         │  reg_no)     │
   ┌──────┴──────┐  └──┬────────┬──┘
   │   consents  │     │ 0..1   │ 0..*
   │ (1 per      │  ┌──┴─────┐ ┌┴──────────────────┐
   │  contact)   │  │company_│ │ company_financials│
   └─────────────┘  │verifi- │ │  (year, turnover) │
                    │cations │ └───────────────────┘
                    └────────┘

   conversations 1──* messages          (channel transport: web / whatsapp)
   conversations 0..1── leads            (a qualified conversation yields one lead)

   documents (POLYMORPHIC):  owner_type ∈ {lead, listing, offer, sourcing_request, ...}
                             owner_id → that row.  Always points to GCS via gcs_key.

   activity_logs (POLYMORPHIC AUDIT): entity_type/entity_id → any row. actor_type/actor_id.

   templates : standalone (email/message templates, RO/EN)

   ── Phase 2 additive ──────────────────────────────────────────────────────
   providers     *──1 companies        (expert directory; seeded from companies.roles[])
   matches       *──1 market_entry_requests (request_id), *──1 providers
   offers        *──1 providers, *──0..1 leads, *──0..1 documents
   lead_terms_acceptance *──1 leads, *──1 providers, *──0..1 users  (clickwrap)
```

**Key cardinality rules**

- `leads (1) → (0..1) sourcing_request | listing | market_entry_request` — a lead references **exactly one**
  typed request (enforced at app level + a partial guard, see §4.6). Adding a vertical = adding a sibling
  table, never altering `leads`.
- `contacts (1) → (1) consents` for the active consent; historical consents are `* per contact`.
- `companies (1) → (0..*) company_verifications`, `(0..*) company_financials` — both nullable/absent for
  foreign or unverified companies.

---

## 2. Extensible enums — design decision (invariant #2)

**Decision: do NOT use native PostgreSQL `ENUM` types.** Native enums require `ALTER TYPE ... ADD VALUE`,
which historically could not run inside a transaction block, cannot be removed/reordered, and turn every
"add a vertical" into a fragile schema migration — directly against invariant #5 ("adding a vertical =
adding a sibling table, no migration of existing data").

We use **two complementary mechanisms**:

1. **Reference / lookup tables** for the high-value, UI-driving enums that benefit from labels, ordering,
   and i18n: `vertical`, `intent`, `lead_status`. Adding a value = one `INSERT`, zero DDL. FKs from the hot
   tables reference these lookups by their stable `code` (a `TEXT` natural key), giving us referential
   integrity *and* extensibility.

2. **`TEXT` columns with a documented allowed-value set** (no `CHECK` pinning the full set) for low-churn,
   write-mostly-by-code enums where a lookup table adds no value: `documents.kind`, `consents.channel`,
   `activity_logs.actor_type`, `users.role`, `matches.status`, `offers.status`, `companies.vat_status`.
   The allowed values are documented inline; the application validates them. We **avoid `CHECK (col IN (...))`
   with the full enumeration** because that reintroduces the "edit DDL to extend" problem; where we *do* use
   a `CHECK`, it is only a coarse, stable invariant (e.g. `year > 1990`, `rank >= 1`).

3. **`companies.roles[]`** is a `TEXT[]` array — see §3.1 for the array-vs-join-table justification.

```sql
-- ── Lookup tables for the UI-driving extensible enums ───────────────────────
CREATE TABLE verticals (
  code        TEXT PRIMARY KEY,              -- 'angrosist','palletclearance','skalyou'
  label_ro    TEXT NOT NULL,
  label_en    TEXT NOT NULL,
  sort_order  INT  NOT NULL DEFAULT 100,
  active      BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO verticals(code,label_ro,label_en,sort_order) VALUES
  ('angrosist','Angrosist','Wholesale buyer',10),
  ('palletclearance','PalletClearance','Pallet clearance',20),
  ('skalyou','SkalYou','Market entry',30);     -- [P2] seeded now, harmless

CREATE TABLE intents (
  code        TEXT PRIMARY KEY,              -- 'buy','sell','market_entry'
  label_ro    TEXT NOT NULL,
  label_en    TEXT NOT NULL,
  active      BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO intents(code,label_ro,label_en) VALUES
  ('buy','Cumpărare','Buy'),
  ('sell','Vânzare','Sell'),
  ('market_entry','Intrare pe piață','Market entry');  -- [P2]

CREATE TABLE lead_statuses (
  code        TEXT PRIMARY KEY,
  label_ro    TEXT NOT NULL,
  label_en    TEXT NOT NULL,
  sort_order  INT  NOT NULL DEFAULT 100,     -- pipeline ordering in the dashboard
  is_terminal BOOLEAN NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO lead_statuses(code,label_ro,label_en,sort_order,is_terminal) VALUES
  ('new','Nou','New',10,false),
  ('qualifying','În calificare','Qualifying',20,false),
  ('needs_human','Necesită operator','Needs human',30,false),
  ('qualified','Calificat','Qualified',40,false),
  ('offer_requested','Ofertă cerută','Offer requested',50,false),
  ('offer_sent','Ofertă trimisă','Offer sent',60,false),
  ('negotiation','Negociere','Negotiation',70,false),
  ('won','Câștigat','Won',80,true),
  ('lost','Pierdut','Lost',90,true);
-- [P2] additive: ('diagnosed',...),('matched',...),('routed',...),('delivered',...),('client_accepted',...)
```

> **Why FK-to-`code` and not FK-to-`id`?** The `code` is the stable contract shared with the LLM agent,
> the dashboard, and analytics. Storing the human-readable code in `leads.status` keeps rows
> self-describing in dumps/logs and avoids a join just to render a list. Referential integrity is still
> guaranteed by the FK.

---

## 3. Phase 1 DDL

### 3.1 companies — the B2B asset (invariant #1, #4)

```sql
CREATE TABLE companies (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  country     TEXT NOT NULL DEFAULT 'RO',     -- ISO-3166-1 alpha-2; generalizes beyond RO
  reg_no      TEXT NOT NULL,                  -- CUI for RO; registration number for foreign cos
  name        TEXT NOT NULL,
  vat_status  TEXT,                           -- 'active'|'inactive'|'not_registered'|NULL (doc'd values)
  caen        TEXT,                           -- RO activity code (nullable for foreign)
  address     TEXT,
  roles       TEXT[] NOT NULL DEFAULT '{}',   -- see allowed values below; seeds P2 provider directory
  -- demo-compat columns kept (forward-only/additive): do NOT drop
  cui         TEXT,                           -- legacy alias == reg_no for RO; kept for demo callers
  county      TEXT,
  is_active   BOOLEAN NOT NULL DEFAULT true,
  raw_data    JSONB,                          -- legacy demo blob; new code uses company_verifications.raw
  verified_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- INVARIANT #1: dedup key. Case-insensitive country, trimmed reg_no recommended at app layer.
CREATE UNIQUE INDEX companies_country_reg_no_uq ON companies (country, reg_no);

-- roles[] membership search (provider directory / role filters). GIN over the array.
CREATE INDEX companies_roles_gin ON companies USING gin (roles);

-- name search for the B2B directory dashboard (trigram; requires pg_trgm).
-- CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX companies_name_trgm ON companies USING gin (name gin_trgm_ops);
```

**`companies.roles[]` — array vs join table (invariant #4).** **Chosen: `TEXT[]` array with a GIN index.**
Roles are a *small, bounded, opportunistic tag set* (≤ 9 values today: distributor, importer, wholesaler,
retailer, horeca, processor, producer, buyer, seller) read almost always as "does this company have role X"
(`roles @> ARRAY['distributor']`) and written as a whole set during qualification. A join table
(`company_roles`) would add a JOIN to every directory query and a second write path for zero relational
benefit — roles carry no attributes of their own. The GIN index makes `@>` / `&&` containment queries fast.
*If* roles later need per-role metadata (e.g. `verified_role_at`, source), promote to a join table additively
(new table, backfill from the array, keep the array as denormalized cache) — that is a clean future migration,
not a reshape of existing columns.

> **Allowed `roles[]` values (documented, not CHECK-pinned):** `distributor`, `importer`, `wholesaler`,
> `retailer`, `horeca`, `processor`, `producer`, `buyer`, `seller`. New roles = code change only.

> **Allowed `vat_status` values:** `active`, `inactive`, `not_registered`, `unknown`, `NULL` (foreign /
> unverified). Nullable by design (invariant #3).

**Query patterns / index rationale:**
- Lookup-on-verify and dedup → `companies_country_reg_no_uq` (also enforces invariant #1).
- Provider directory & role filters (P2 matching seed) → `companies_roles_gin`.
- Dashboard "B2B DB" name search → `companies_name_trgm`.

### 3.2 company_verifications — nullable, RO/DemoANAF-specific (invariant #3)

```sql
CREATE TABLE company_verifications (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id     UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  source         TEXT NOT NULL DEFAULT 'demoanaf',   -- doc'd: 'demoanaf' (extensible: 'vies', ...)
  vat_status     TEXT,
  administrators JSONB,                                -- array of {name, role} from DemoANAF
  raw            JSONB,                                -- full DemoANAF /company/:cui payload (cache)
  checked_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX company_verifications_company_checked_idx
  ON company_verifications (company_id, checked_at DESC);   -- latest verification per company
-- GIN only if we query inside the payload (e.g. administrator search). Enable on demand:
-- CREATE INDEX company_verifications_raw_gin ON company_verifications USING gin (raw jsonb_path_ops);
```

**Query patterns:** "latest verification for company X" → composite `(company_id, checked_at DESC)`.
`raw` is JSONB so we can cache the whole DemoANAF response (FR-4.3) and re-derive fields later without a
re-fetch. GIN on `raw` only if we ever filter *inside* it — noted, not created by default (write cost).

### 3.3 company_financials — nullable, optional (invariant #3)

```sql
CREATE TABLE company_financials (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  year        INT  NOT NULL CHECK (year > 1990 AND year < 2100),
  turnover    NUMERIC(16,2),
  raw         JSONB,                                  -- DemoANAF /financials payload
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (company_id, year)                           -- one financial record per company per year
);

CREATE INDEX company_financials_company_year_idx
  ON company_financials (company_id, year DESC);
```

**Query patterns:** show last N years of turnover for a company → `(company_id, year DESC)`. `UNIQUE` keeps
re-fetches idempotent (upsert on `(company_id, year)`).

### 3.4 contacts (personal data — GDPR sensitive)

```sql
CREATE TABLE contacts (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id  UUID REFERENCES companies(id) ON DELETE SET NULL,  -- company survives contact erasure
  name        TEXT,
  phone       TEXT,                                  -- personal data
  email       TEXT,                                  -- personal data
  language    TEXT NOT NULL DEFAULT 'ro',            -- 'ro'|'en' (FR-2.6)
  consent_id  UUID,                                  -- FK added after consents exists (§3.10)
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX contacts_company_idx ON contacts (company_id);
CREATE INDEX contacts_phone_idx   ON contacts (phone);   -- WhatsApp inbound lookup by sender number
CREATE INDEX contacts_email_idx   ON contacts (email);
```

**GDPR note:** the contact is the root of the personal-data graph. Erasing a contact triggers the cascade
in §4. `company_id` is `ON DELETE SET NULL` *in the reverse direction is N/A* — here it means if a company is
ever deleted (rare), contacts keep existing with a null company. The personal→company separation
(NFR-3) is structural: company data lives in `companies` (public), personal data lives in `contacts`.

**Query patterns:** inbound WhatsApp message resolves a contact by `phone`; widget by session→contact;
dashboard lists contacts per company.

### 3.5 categories — shared hierarchical taxonomy (matching enabler)

```sql
CREATE TABLE categories (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id  UUID REFERENCES categories(id) ON DELETE RESTRICT,  -- protect the tree
  name       TEXT NOT NULL,
  code       TEXT NOT NULL UNIQUE,                  -- stable slug used by agent + matching
  sort_order INT  NOT NULL DEFAULT 100,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX categories_parent_idx ON categories (parent_id);
```

`ON DELETE RESTRICT` on `parent_id` prevents orphaning a subtree by accident; deletions must re-parent first.

### 3.6 leads — the THIN EVENT (invariant #5)

```sql
CREATE TABLE leads (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE,  -- demo: present + UNIQUE
  contact_id      UUID REFERENCES contacts(id) ON DELETE CASCADE,       -- GDPR cascade root path
  company_id      UUID REFERENCES companies(id) ON DELETE SET NULL,     -- keep lead, drop company link
  assigned_to     UUID REFERENCES users(id) ON DELETE SET NULL,         -- unassign on user deletion
  vertical        TEXT NOT NULL REFERENCES verticals(code),             -- extensible via lookup
  intent          TEXT NOT NULL REFERENCES intents(code),               -- extensible via lookup
  source          TEXT,                              -- channel/site/number (e.g. 'web:angrosist', 'wa:+40...')
  status          TEXT NOT NULL DEFAULT 'new' REFERENCES lead_statuses(code),
  summary         TEXT,
  needs_human     BOOLEAN NOT NULL DEFAULT false,    -- FR-6.1 escalation flag
  bot_active      BOOLEAN NOT NULL DEFAULT true,     -- FR-6.1/6.3 mute bot on handoff
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- demo invariant preserved: one lead per conversation
CREATE UNIQUE INDEX leads_conversation_uq ON leads (conversation_id) WHERE conversation_id IS NOT NULL;

-- HOT: pipeline list. Most dashboard queries filter by status + vertical, order by recency.
CREATE INDEX leads_pipeline_idx      ON leads (vertical, status, created_at DESC);
CREATE INDEX leads_assigned_idx      ON leads (assigned_to, status) WHERE assigned_to IS NOT NULL;
CREATE INDEX leads_created_idx       ON leads (created_at DESC);
CREATE INDEX leads_contact_idx       ON leads (contact_id);     -- GDPR cascade + contact timeline
CREATE INDEX leads_company_idx       ON leads (company_id);
-- HOT: handoff queue — only the rows that need a human. Partial index keeps it tiny.
CREATE INDEX leads_needs_human_idx   ON leads (created_at DESC) WHERE needs_human = true;
```

**Why no `request_id`/`request_type` columns on `leads`?** Putting a polymorphic FK on the lead would mean
every new vertical edits `leads` (a `CHECK` or a new column) — violating invariant #5. Instead the **typed
request owns the FK** (`sourcing_requests.lead_id`, `listings.lead_id`, `market_entry_requests.lead_id`),
each `UNIQUE`. A lead therefore has at most one row in *each* sibling table, and the app guarantees exactly
one across all of them. Adding SkalYou = adding `market_entry_requests` only; `leads` is untouched. See §4.6
for the "exactly one typed request" guard.

**Query patterns / index rationale:**
- Pipeline board (FR-7.1): filter `vertical` + `status`, newest first → `leads_pipeline_idx`.
- "My leads" per staff → `leads_assigned_idx` (partial, only assigned).
- Handoff queue (FR-6.2, FR-7.5) → `leads_needs_human_idx` **partial** — indexes only escalated rows, so
  the queue scan never touches the (large) qualified/won/lost majority.
- GDPR erasure & contact timeline → `leads_contact_idx`.

### 3.7 sourcing_requests — Angrosist buyer demand (typed request #1)

```sql
CREATE TABLE sourcing_requests (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id           UUID NOT NULL UNIQUE REFERENCES leads(id) ON DELETE CASCADE,  -- one per lead
  category_id       UUID REFERENCES categories(id) ON DELETE SET NULL,
  product           TEXT,                            -- canonical; demo kept `product_name`
  product_name      TEXT,                            -- demo-compat (do NOT drop)
  quantity          NUMERIC(16,3),
  unit              TEXT,                             -- demo column kept
  delivery_location TEXT,
  recurring         BOOLEAN NOT NULL DEFAULT false,
  deadline          DATE,
  budget            NUMERIC(14,2),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sourcing_requests_category_idx ON sourcing_requests (category_id);
-- lead_id already indexed by its UNIQUE constraint.
```

`ON DELETE CASCADE` from `lead_id`: when a lead is erased, its sourcing request goes with it (GDPR cascade).

### 3.8 listings — PalletClearance supply / clearance (typed request #2)

```sql
CREATE TABLE listings (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id      UUID NOT NULL UNIQUE REFERENCES leads(id) ON DELETE CASCADE,  -- one per lead
  company_id   UUID REFERENCES companies(id) ON DELETE SET NULL,             -- seller company
  category_id  UUID REFERENCES categories(id) ON DELETE SET NULL,
  stock_type   TEXT,
  quantity     NUMERIC(16,3),
  location     TEXT,
  country      TEXT,                                 -- multi-country filter (NFR-1)
  expiry       DATE,
  target_price NUMERIC(14,2),
  confidential BOOLEAN NOT NULL DEFAULT false,
  status       TEXT NOT NULL DEFAULT 'active',       -- doc'd: 'active'|'reserved'|'sold'|'expired'|'withdrawn'
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Inventory dashboard: filter by country + status, surface soonest-expiring first.
CREATE INDEX listings_country_status_idx ON listings (country, status);
CREATE INDEX listings_expiry_idx         ON listings (expiry) WHERE status = 'active';  -- near-expiry feed
CREATE INDEX listings_category_idx       ON listings (category_id);
CREATE INDEX listings_company_idx        ON listings (company_id);
```

> Seller photos are **mandatory** (invariant: flow blocks until photos uploaded). The photos live in
> `documents` (`owner_type='listing'`); the *flow engine* enforces presence before the lead is submitted —
> the DB does not (a count-based trigger would couple schema to flow). Documented as an app-layer invariant.

**Query patterns:** inventory list filtered by `country`+`status` → `listings_country_status_idx`;
near-expiry feed (FR-3.2 buyer subscription) → `listings_expiry_idx` partial on active only.

### 3.9 buyer_profiles — standing demand (PalletClearance buyer feed)

```sql
CREATE TABLE buyer_profiles (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id    UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  vertical      TEXT NOT NULL REFERENCES verticals(code),
  categories    UUID[] NOT NULL DEFAULT '{}',        -- category ids of interest (matching)
  countries     TEXT[] NOT NULL DEFAULT '{}',        -- ISO codes of interest
  near_expiry_ok BOOLEAN NOT NULL DEFAULT false,
  subscribed    BOOLEAN NOT NULL DEFAULT true,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX buyer_profiles_company_idx     ON buyer_profiles (company_id);
CREATE INDEX buyer_profiles_categories_gin  ON buyer_profiles USING gin (categories);
CREATE INDEX buyer_profiles_countries_gin   ON buyer_profiles USING gin (countries);
CREATE INDEX buyer_profiles_subscribed_idx  ON buyer_profiles (subscribed) WHERE subscribed = true;
```

**Query patterns:** when a new `listing` appears, find subscribed buyers whose `categories` / `countries`
overlap → GIN `@>`/`&&` on the arrays, filtered by the partial `subscribed` index.

### 3.10 consents — first-class (invariant #7, NFR-3)

```sql
CREATE TABLE consents (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  contact_id   UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  text_version TEXT NOT NULL,                        -- which consent text/version was shown
  given_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  channel      TEXT NOT NULL,                        -- doc'd: 'web'|'whatsapp'
  ip           INET,                                 -- captured at consent (proof)
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX consents_contact_idx ON consents (contact_id, given_at DESC);

-- Deferred FK: contacts.consent_id -> consents.id (active consent pointer). Added after both tables exist.
ALTER TABLE contacts
  ADD CONSTRAINT contacts_consent_fk
  FOREIGN KEY (consent_id) REFERENCES consents(id) ON DELETE SET NULL;
```

> `contacts.consent_id` points at the *current* consent; `consents` keeps the full history per contact. The
> circular dependency (contact↔consent) is resolved with a **deferred `ALTER TABLE`** after both tables
> exist — the standard pattern; do not try to inline it.

### 3.11 conversations & messages (channel transport — already in demo)

```sql
-- conversations: demo table, extended additively.
CREATE TABLE conversations (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  channel     TEXT NOT NULL DEFAULT 'web',          -- 'web'|'whatsapp'
  state       TEXT NOT NULL DEFAULT 'greeting',     -- flow-engine state machine label (free-form by design)
  extracted   JSONB NOT NULL DEFAULT '{}',          -- partial extracted fields during qualification
  contact_id  UUID REFERENCES contacts(id) ON DELETE CASCADE,   -- [additive] GDPR cascade path
  language    TEXT,                                  -- [additive] detected language
  bot_active  BOOLEAN NOT NULL DEFAULT true,         -- [additive] handoff mute at conversation level
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX conversations_contact_idx ON conversations (contact_id);

-- messages: demo table, already correctly shaped + indexed.
CREATE TABLE messages (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  role            TEXT NOT NULL,                     -- 'user'|'assistant'|'tool'|'system'
  content         TEXT,
  tool_calls      JSONB,
  tool_call_id    TEXT,
  provider_msg_id TEXT,                              -- [additive] WhatsApp message id (idempotency/dedupe)
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- HOT: transcript render — every message for a conversation in order.
CREATE INDEX messages_conversation_created_idx ON messages (conversation_id, created_at);
-- Idempotency (FR-2.4): dedupe inbound WhatsApp by provider message id.
CREATE UNIQUE INDEX messages_provider_msg_uq
  ON messages (provider_msg_id) WHERE provider_msg_id IS NOT NULL;
```

**Query patterns:** transcript view (FR-7.1) → `messages_conversation_created_idx`; inbound dedupe (FR-2.4)
→ partial unique on `provider_msg_id`.

### 3.12 documents — polymorphic, always GCS (invariant #6)

```sql
CREATE TABLE documents (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type  TEXT NOT NULL,                         -- 'lead'|'listing'|'offer'|'sourcing_request' (doc'd, extensible)
  owner_id    UUID NOT NULL,                         -- soft polymorphic FK (no DB FK across types)
  kind        TEXT NOT NULL,                         -- 'photo'|'product_list'|'offer' (doc'd, extensible)
  gcs_key     TEXT NOT NULL,                         -- INVARIANT #6: files always in GCS, never on disk
  mime        TEXT,
  size_bytes  BIGINT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- HOT: "all documents for this lead/listing/offer".
CREATE INDEX documents_owner_idx ON documents (owner_type, owner_id);
```

> **Polymorphic owner = no cross-table DB FK.** A single `(owner_type, owner_id)` pair cannot be a Postgres
> FK to multiple tables. Integrity is enforced in app code. **Consequence for GDPR:** because there is no
> `ON DELETE CASCADE` from the owner row to `documents`, document deletion is an **app-code step** in the
> erasure path — and it must happen there anyway because the **GCS object** behind `gcs_key` has to be
> deleted via the GCS API; a DB cascade alone would orphan the blob. See §4.

### 3.13 activity_logs / audit — first-class (invariant #7, NFR-3)

```sql
CREATE TABLE activity_logs (
  id          BIGSERIAL PRIMARY KEY,                 -- high-volume append-only; bigserial is fine
  actor_type  TEXT NOT NULL,                         -- 'agent'|'staff'|'provider'|'system' (doc'd)
  actor_id    UUID,                                  -- nullable for 'system'/'agent'
  action      TEXT NOT NULL,                         -- e.g. 'lead.created','company.verified','offer.viewed'
  entity_type TEXT,                                  -- polymorphic subject: 'lead'|'company'|'offer'|...
  entity_id   UUID,
  meta        JSONB NOT NULL DEFAULT '{}',           -- structured payload (request ids, diffs, ip, etc.)
  redacted    BOOLEAN NOT NULL DEFAULT false,        -- GDPR: set true when source personal data erased
  at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- HOT: audit trail for an entity, newest first.
CREATE INDEX activity_logs_entity_idx ON activity_logs (entity_type, entity_id, at DESC);
CREATE INDEX activity_logs_actor_idx  ON activity_logs (actor_type, actor_id, at DESC);
CREATE INDEX activity_logs_at_idx     ON activity_logs (at DESC);
-- Query inside meta (e.g. find logs for a given lead embedded in payload):
CREATE INDEX activity_logs_meta_gin   ON activity_logs USING gin (meta jsonb_path_ops);
```

**GDPR for audit:** audit must survive erasure (legal/anti-circumvention requirement, FR-9.8), so logs are
**not deleted** when a contact is erased — instead they are **anonymized/redacted** (`redacted=true`, PII
fields stripped from `meta`, `actor_id`/`entity_id` retained or nulled per policy). See §4.

**Query patterns:** entity history → `activity_logs_entity_idx`; per-actor audit → `activity_logs_actor_idx`;
search within payload → `activity_logs_meta_gin` (use `jsonb_path_ops` — smaller/faster for `@>` containment).

### 3.14 users & roles

```sql
CREATE TABLE users (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email      TEXT NOT NULL UNIQUE,
  name       TEXT,
  role       TEXT NOT NULL DEFAULT 'staff',          -- P1: 'staff'|'admin'; P2: 'provider','admin_global','country_operator'
  active     BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- email already unique-indexed by the constraint.
```

`role` is a documented `TEXT` set (not an enum type) precisely so P2 roles (`provider`, `admin_global`,
`country_operator`) are added with zero DDL.

### 3.15 templates

```sql
CREATE TABLE templates (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code       TEXT NOT NULL,                          -- stable key, e.g. 'lead_confirmation'
  channel    TEXT NOT NULL,                          -- 'email'|'whatsapp'
  language   TEXT NOT NULL,                          -- 'ro'|'en'
  subject    TEXT,                                   -- email only
  body       TEXT NOT NULL,
  active     BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (code, channel, language)                   -- one active template per (code,channel,lang)
);
CREATE INDEX templates_lookup_idx ON templates (code, channel, language) WHERE active = true;
```

---

## 4. GDPR cascade erasure design (invariant #7, NFR-3)

**Goal:** "right to erasure" deletes a contact and everything personal that hangs off it, returns the set of
**GCS keys** to delete out-of-band, and **anonymizes** (does not delete) the audit trail.

### 4.1 Cascade graph and ON DELETE behavior

```
DELETE contact (root)
  └─(ON DELETE CASCADE)→ consents              -- contacts.id ← consents.contact_id
  └─(ON DELETE CASCADE)→ conversations         -- contacts.id ← conversations.contact_id
  │      └─(ON DELETE CASCADE)→ messages       -- conversations.id ← messages.conversation_id
  └─(ON DELETE CASCADE)→ leads                 -- contacts.id ← leads.contact_id
         └─(ON DELETE CASCADE)→ sourcing_requests   -- leads.id ← sourcing_requests.lead_id
         └─(ON DELETE CASCADE)→ listings            -- leads.id ← listings.lead_id
         └─(ON DELETE CASCADE)→ market_entry_requests [P2]
         └─(ON DELETE CASCADE)→ lead_terms_acceptance [P2]
         └─(ON DELETE SET NULL on offers.lead_id)   [P2]  -- offer record kept, lead link nulled

NOT auto-cascaded — handled in app code:
  • documents       -- polymorphic (owner_type/owner_id); no DB FK. MUST delete GCS object first.
  • GCS blobs       -- live outside Postgres; deleted via FileStore port.
  • activity_logs   -- ANONYMIZED, never deleted (FR-9.8 anti-circumvention / legal retention).
  • companies       -- public/legal-entity data, NOT personal; survives. leads.company_id = SET NULL only
                       if the company itself is removed (it normally isn't on contact erasure).
```

| FK | On delete | Why |
|---|---|---|
| `consents.contact_id → contacts` | **CASCADE** | consent is personal, dies with the contact |
| `conversations.contact_id → contacts` | **CASCADE** | conversation is personal context |
| `messages.conversation_id → conversations` | **CASCADE** | transcript content is personal |
| `leads.contact_id → contacts` | **CASCADE** | lead is the personal event |
| `leads.conversation_id → conversations` | **CASCADE** | demo path; conversation gone → lead gone |
| `leads.company_id → companies` | **SET NULL** | company is public; lead may stay if erasing only company |
| `leads.assigned_to → users` | **SET NULL** | don't lose the lead when a staff user is removed |
| `sourcing_requests.lead_id → leads` | **CASCADE** | typed request dies with its lead |
| `listings.lead_id → leads` | **CASCADE** | typed request dies with its lead |
| `listings.company_id → companies` | **SET NULL** | keep listing if company link removed |
| `company_verifications.company_id → companies` | **CASCADE** | verification meaningless without company |
| `company_financials.company_id → companies` | **CASCADE** | same |
| `buyer_profiles.company_id → companies` | **CASCADE** | profile belongs to company |
| `categories.parent_id → categories` | **RESTRICT** | protect the taxonomy tree |
| `contacts.company_id → companies` | **SET NULL** | contact keeps existing if company removed |
| `documents` (polymorphic) | **app-code** | no DB FK; GCS blob must be deleted via API |
| `activity_logs` | **app-code (anonymize)** | retained for audit/anti-circumvention; redacted not deleted |

### 4.2 Erasure procedure (app-code, in a single DB transaction + a GCS step)

1. **Collect GCS keys first.** Query `documents` for every owner reachable from the contact
   (`leads`, their `listings`/`sourcing_requests`, `offers`) **before** deleting anything:
   ```sql
   SELECT gcs_key FROM documents
   WHERE (owner_type, owner_id) IN (
     SELECT 'lead', l.id FROM leads l WHERE l.contact_id = $1
     UNION ALL SELECT 'sourcing_request', s.id FROM sourcing_requests s
        JOIN leads l ON l.id = s.lead_id WHERE l.contact_id = $1
     UNION ALL SELECT 'listing', li.id FROM listings li
        JOIN leads l ON l.id = li.lead_id WHERE l.contact_id = $1
     -- [P2] UNION ALL offers ...
   );
   ```
2. **Delete those `documents` rows** (no DB cascade reaches them).
3. **Anonymize `activity_logs`**: `UPDATE activity_logs SET redacted=true, meta = meta - 'pii_keys...'`
   for rows referencing the erased entities; keep the row for audit integrity.
4. **`DELETE FROM contacts WHERE id=$1`** — the FK CASCADE chain removes consents, conversations→messages,
   leads→typed requests automatically (one statement).
5. **Commit the transaction.**
6. **After commit, delete the collected GCS objects** via the `FileStore` port (idempotent; safe to retry).
   GCS deletion is *outside* the DB transaction by necessity — record the operation in `activity_logs`
   (`action='gdpr.erasure'`, `meta={contact_id, gcs_keys_deleted}`).

> **Why GCS deletion is app-code, not a trigger:** Postgres cannot delete an object in Cloud Storage. A DB
> cascade that removed `documents` rows would leave the blobs orphaned in GCS — a GDPR failure. Therefore the
> erasure use-case *must* run in the domain layer: read keys → delete DB rows → delete GCS blobs → audit.

---

## 5. Phase 2 DDL (ADDITIVE — not created in Phase 1; schema already allows)

> Everything below is **new tables and new nullable columns**. No Phase-1 table is reshaped. These ship
> when SkalYou (FR-9) is built.

### 5.1 market_entry_requests — SkalYou typed request (sibling #3 of the lead)

```sql
CREATE TABLE market_entry_requests (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id             UUID NOT NULL UNIQUE REFERENCES leads(id) ON DELETE CASCADE,  -- one per lead
  company_id          UUID REFERENCES companies(id) ON DELETE SET NULL,
  category_id         UUID REFERENCES categories(id) ON DELETE SET NULL,
  partner_types       TEXT[] NOT NULL DEFAULT '{}',   -- distributor|buyer|importer|retail|horeca|processor
  has_permanent_stock BOOLEAN,
  active_markets      TEXT[] NOT NULL DEFAULT '{}',    -- ISO country codes
  target_market       TEXT,                            -- ISO country code
  website             TEXT,
  objective           TEXT,
  specialist_type     TEXT,                            -- classified expert type
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mer_target_market_idx   ON market_entry_requests (target_market);
CREATE INDEX mer_partner_types_gin   ON market_entry_requests USING gin (partner_types);
CREATE INDEX mer_active_markets_gin  ON market_entry_requests USING gin (active_markets);
CREATE INDEX mer_category_idx        ON market_entry_requests (category_id);
```

**This is the proof of invariant #5:** adding the SkalYou vertical = creating this one table. `leads` is not
altered. The cascade in §4 already lists it.

### 5.2 providers — SkalYou expert directory (extends companies/users)

```sql
CREATE TABLE providers (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id       UUID REFERENCES companies(id) ON DELETE SET NULL,   -- seeded from companies.roles[]
  user_id          UUID REFERENCES users(id) ON DELETE SET NULL,        -- auth identity (Google/OTP)
  auth_provider    TEXT,                                                -- 'google'|'otp'
  categories       UUID[] NOT NULL DEFAULT '{}',                        -- category ids served
  roles            TEXT[] NOT NULL DEFAULT '{}',                        -- same vocabulary as companies.roles
  markets          TEXT[] NOT NULL DEFAULT '{}',                        -- ISO country codes served
  plan             TEXT NOT NULL DEFAULT 'starter',                     -- 'starter'|'pro'|'vip' (monetization field)
  consent_to_leads BOOLEAN NOT NULL DEFAULT false,                      -- FR-9.2 consent to receive leads
  quality_score    NUMERIC(5,2),                                        -- nullable, P2-later
  active           BOOLEAN NOT NULL DEFAULT true,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Matching: find active providers by category ∩ role ∩ market.
CREATE INDEX providers_categories_gin ON providers USING gin (categories);
CREATE INDEX providers_roles_gin      ON providers USING gin (roles);
CREATE INDEX providers_markets_gin    ON providers USING gin (markets);
CREATE INDEX providers_active_idx     ON providers (active) WHERE active = true;
CREATE INDEX providers_company_idx    ON providers (company_id);
```

### 5.3 matches — routing of a request to providers (FR-9.1, FR-9.5)

```sql
CREATE TABLE matches (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id   UUID NOT NULL REFERENCES market_entry_requests(id) ON DELETE CASCADE,
  provider_id  UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  rank         INT  NOT NULL CHECK (rank >= 1),       -- best=1; "best → timeout → next"
  status       TEXT NOT NULL DEFAULT 'routed',        -- 'routed'|'accepted'|'declined'|'expired' (doc'd)
  attempt_no   INT  NOT NULL DEFAULT 1,               -- after 3 attempts → escalate to staff
  routed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  responded_at TIMESTAMPTZ,
  UNIQUE (request_id, provider_id)                    -- a provider is matched to a request once
);
-- Cron timeouts (FR-9.5): find routed matches past their accept window.
CREATE INDEX matches_status_routed_idx ON matches (status, routed_at) WHERE status = 'routed';
CREATE INDEX matches_request_rank_idx  ON matches (request_id, rank);
CREATE INDEX matches_provider_idx      ON matches (provider_id, status);
```

### 5.4 offers — manual offer tracking + provider offers (FR-7.3, FR-9.2)

```sql
CREATE TABLE offers (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id              UUID REFERENCES leads(id) ON DELETE SET NULL,          -- keep offer record on lead erase
  request_id           UUID REFERENCES market_entry_requests(id) ON DELETE SET NULL,
  provider_id          UUID REFERENCES providers(id) ON DELETE SET NULL,
  document_id          UUID REFERENCES documents(id) ON DELETE SET NULL,      -- the offer file in GCS
  value                NUMERIC(14,2),                  -- offer value
  status               TEXT NOT NULL DEFAULT 'uploaded', -- uploaded|delivered|accepted|clarification_requested
  -- monetization (data only, no billing logic) ──────────────────────────────
  commission           NUMERIC(6,3),                   -- applicable %
  estimated_commission NUMERIC(14,2),
  transaction_status   TEXT NOT NULL DEFAULT 'uninvoiced', -- uninvoiced|invoiced|paid|cancelled
  lead_accepted_at     TIMESTAMPTZ,                    -- client accepted the lead/offer
  delivered_at         TIMESTAMPTZ,
  client_action_at     TIMESTAMPTZ,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX offers_lead_idx        ON offers (lead_id);
CREATE INDEX offers_request_idx     ON offers (request_id);
CREATE INDEX offers_provider_idx    ON offers (provider_id, status);
CREATE INDEX offers_status_idx      ON offers (status);
CREATE INDEX offers_txn_status_idx  ON offers (transaction_status);
```

> Offers are intentionally **`SET NULL` on lead erasure**, not `CASCADE`: the offer/transaction record is a
> business/financial artifact that may need to survive a personal-data erasure (the lead link is nulled, the
> commercial fact remains). The offer **document** still gets GCS-erased via the §4 procedure when the lead
> is erased, since `documents` is handled in app code.

### 5.5 lead_terms_acceptance — clickwrap / anti-leakage (FR-9.3, FR-9.8)

```sql
CREATE TABLE lead_terms_acceptance (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id       UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
  provider_id   UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  terms_version TEXT NOT NULL,
  accepted_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  ip            INET NOT NULL,                          -- proof; FR-9.3 logs IP
  user_id       UUID REFERENCES users(id) ON DELETE SET NULL,  -- who accepted (provider user)
  UNIQUE (lead_id, provider_id, terms_version)
);
CREATE INDEX lta_lead_idx     ON lead_terms_acceptance (lead_id);
CREATE INDEX lta_provider_idx ON lead_terms_acceptance (provider_id);
```

> **Clickwrap invariant:** a provider must have a `lead_terms_acceptance` row (timestamp+IP+user+version)
> **before** any client personal data is shown. Enforced in the API/use-case layer (the dashboard/portal
> checks for the row); the table is the durable, audited proof. Recorded additionally in `activity_logs`
> (`action='lead.terms_accepted'`).

### 5.6 Phase-2 additive columns on Phase-1 tables (no reshape)

```sql
-- Multi-country / market tags (NFR-1, FR-9.6) — all nullable, additive.
ALTER TABLE companies ADD COLUMN IF NOT EXISTS market_tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE leads     ADD COLUMN IF NOT EXISTS target_market TEXT;   -- ISO country
-- contacts already carry language; users gain country for country_operator role later:
ALTER TABLE users     ADD COLUMN IF NOT EXISTS country TEXT;
CREATE INDEX IF NOT EXISTS companies_market_tags_gin ON companies USING gin (market_tags);
CREATE INDEX IF NOT EXISTS leads_target_market_idx   ON leads (target_market) WHERE target_market IS NOT NULL;
-- New P2 lead statuses are INSERTs into lead_statuses (no DDL) — see §2.
```

---

## 6. Index summary — hot query paths (performance, NFR-5)

| Path | Table | Index | Type / note |
|---|---|---|---|
| Pipeline board (vertical+status, recent) | `leads` | `leads_pipeline_idx (vertical,status,created_at DESC)` | composite |
| "My leads" per staff | `leads` | `leads_assigned_idx (assigned_to,status)` | **partial** `assigned_to IS NOT NULL` |
| Handoff queue | `leads` | `leads_needs_human_idx (created_at DESC)` | **partial** `needs_human=true` |
| Lead by contact (GDPR + timeline) | `leads` | `leads_contact_idx` | btree |
| One lead per conversation | `leads` | `leads_conversation_uq` | **partial unique** |
| Company dedup / verify lookup | `companies` | `companies_country_reg_no_uq` | **unique**, invariant #1 |
| Provider directory / role filter | `companies` | `companies_roles_gin` | **GIN** on `roles[]` |
| B2B DB name search | `companies` | `companies_name_trgm` | **GIN trgm** |
| Latest verification | `company_verifications` | `(company_id,checked_at DESC)` | composite |
| Transcript render | `messages` | `messages_conversation_created_idx` | composite |
| Inbound dedupe | `messages` | `messages_provider_msg_uq` | **partial unique** |
| Documents by owner | `documents` | `documents_owner_idx (owner_type,owner_id)` | composite |
| Audit by entity | `activity_logs` | `activity_logs_entity_idx` | composite |
| Audit payload search | `activity_logs` | `activity_logs_meta_gin (meta jsonb_path_ops)` | **GIN** |
| Inventory by country+status | `listings` | `listings_country_status_idx` | composite |
| Near-expiry feed | `listings` | `listings_expiry_idx` | **partial** `status='active'` |
| Buyer feed matching | `buyer_profiles` | `categories_gin`,`countries_gin`,`subscribed`(partial) | **GIN** + partial |
| Provider matching [P2] | `providers` | `categories_gin`,`roles_gin`,`markets_gin` | **GIN** |
| Match timeouts (cron) [P2] | `matches` | `matches_status_routed_idx` | **partial** `status='routed'` |

**JSONB / GIN policy:** GIN indexes are created **only** where we query *inside* the payload
(`activity_logs.meta`). `company_verifications.raw` and `company_financials.raw` are JSONB **caches** we read
by `company_id` (already indexed) and rarely filter internally — their GIN indexes are documented but
**not created by default** to avoid write amplification. Prefer `jsonb_path_ops` for containment-only GIN.

---

## 7. Migration discipline (invariant #8)

- **Forward-only.** Every change is a new numbered file in `backend/migrations/` (`009_...`, `010_...`).
  No down-migrations are relied upon in production; roll forward.
- **Additive by default.** Add columns (nullable or with defaults) and new tables. **Never** `DROP COLUMN`,
  `RENAME COLUMN`, or change the type of a populated `companies` / `contacts` / `leads` / `sourcing_requests`
  column. Where the canonical name differs from the demo (`reg_no` vs `cui`, `product` vs `product_name`),
  **keep both**; the repository writes/reads the canonical column, the demo column is backfilled and retained.
- **Backfill in the same migration** when adding a NOT NULL column to a populated table: add nullable → backfill
  → set NOT NULL / add constraint, all forward.
- **Lookup tables over enums** (§2): extending `vertical`/`intent`/`status` is an `INSERT`, not a migration.
- **Adding a vertical = one new sibling table** (`market_entry_requests` is the worked example) — `leads`
  is never altered for a new vertical.
- **Indexes concurrently in prod.** Create hot-path indexes with `CREATE INDEX CONCURRENTLY` outside a txn on
  populated tables to avoid write locks (omitted from the DDL above for readability; apply in the migration).
- **EU residency.** All of this runs on Cloud SQL in `europe-*`; no schema choice changes that, but no data
  may be copied to a non-EU store.
```
