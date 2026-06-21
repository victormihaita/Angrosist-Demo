-- 016_leads_enrich.sql
-- Concern: enrich the demo `leads` THIN EVENT to the Phase-1 target (§3.6, invariants #5, #7).
-- Demo leads: id, conversation_id (NOT NULL, UNIQUE), company_id, contact_id, status, created_at.
-- We ADD: assigned_to, vertical, intent, source, summary, needs_human, bot_active, updated_at.
-- We DO NOT add request_id/request_type -- the typed request owns the FK (invariant #5).
-- We backfill vertical='angrosist', intent='buy' for existing demo rows (the only demo vertical),
-- then enforce NOT NULL + FK to the lookup tables (010).
-- status gains an FK to lead_statuses(code); demo default 'new' already valid.
-- GDPR FK corrections (§4.1):
--   conversation_id -> CASCADE (demo path; conversation gone => lead gone)
--   contact_id      -> CASCADE (lead is the personal event)
--   company_id      -> SET NULL (company is public; keep lead)
--   assigned_to     -> SET NULL (don't lose lead when staff user removed)
-- Demo `conversation_id` stays NOT NULL with its UNIQUE constraint (one lead per conversation kept).
-- We ALSO add the spec's partial unique (harmless alongside the demo non-partial unique).
-- Forward-only/additive: new columns + index + FK behavior corrections (no data reshape).
-- Rollback note: drop the added columns/indexes; revert FK constraints to NO ACTION.

-- 1) Additive columns (nullable / defaulted).
ALTER TABLE leads ADD COLUMN IF NOT EXISTS assigned_to UUID;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS vertical    TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS intent      TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS source      TEXT;     -- e.g. 'web:angrosist', 'wa:+40...'
ALTER TABLE leads ADD COLUMN IF NOT EXISTS summary     TEXT;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS needs_human BOOLEAN NOT NULL DEFAULT false;  -- FR-6.1 escalation
ALTER TABLE leads ADD COLUMN IF NOT EXISTS bot_active  BOOLEAN NOT NULL DEFAULT true;   -- FR-6.1/6.3 mute on handoff
ALTER TABLE leads ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT now();

-- 2) Backfill the extensible-enum columns for existing demo rows (Angrosist buy is the demo vertical).
UPDATE leads SET vertical = 'angrosist' WHERE vertical IS NULL;
UPDATE leads SET intent   = 'buy'       WHERE intent   IS NULL;

-- 3) Defaults + NOT NULL + FK to lookup tables now that data is backfilled.
ALTER TABLE leads ALTER COLUMN vertical SET NOT NULL;
ALTER TABLE leads ALTER COLUMN intent   SET NOT NULL;
ALTER TABLE leads ALTER COLUMN status   SET DEFAULT 'new';

ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_vertical_fk;
ALTER TABLE leads ADD CONSTRAINT leads_vertical_fk
  FOREIGN KEY (vertical) REFERENCES verticals(code);          -- RESTRICT (taxonomy default)
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_intent_fk;
ALTER TABLE leads ADD CONSTRAINT leads_intent_fk
  FOREIGN KEY (intent) REFERENCES intents(code);              -- RESTRICT (taxonomy default)
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_status_fk;
ALTER TABLE leads ADD CONSTRAINT leads_status_fk
  FOREIGN KEY (status) REFERENCES lead_statuses(code);        -- RESTRICT (taxonomy default)

-- 4) assigned_to FK -> users, SET NULL on user deletion (§4.1).
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_assigned_to_fkey;
ALTER TABLE leads ADD CONSTRAINT leads_assigned_to_fkey
  FOREIGN KEY (assigned_to) REFERENCES users(id) ON DELETE SET NULL;

-- 5) Correct GDPR ON DELETE behavior on the demo FKs (constraint re-point; data untouched).
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_conversation_id_fkey;
ALTER TABLE leads ADD CONSTRAINT leads_conversation_id_fkey
  FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE;  -- §4.1
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_contact_id_fkey;
ALTER TABLE leads ADD CONSTRAINT leads_contact_id_fkey
  FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE;           -- §4.1 (personal event)
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_company_id_fkey;
ALTER TABLE leads ADD CONSTRAINT leads_company_id_fkey
  FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE SET NULL;         -- §4.1 (company public)

-- 6) Hot-path indexes (§3.6 / §6).
-- Pipeline board: filter vertical + status, newest first.
CREATE INDEX IF NOT EXISTS leads_pipeline_idx ON leads (vertical, status, created_at DESC);
-- "My leads" per staff (partial: only assigned rows).
CREATE INDEX IF NOT EXISTS leads_assigned_idx ON leads (assigned_to, status) WHERE assigned_to IS NOT NULL;
-- Generic recency listing.
CREATE INDEX IF NOT EXISTS leads_created_idx ON leads (created_at DESC);
-- GDPR cascade + contact timeline.
CREATE INDEX IF NOT EXISTS leads_contact_idx ON leads (contact_id);
-- Lead-by-company.
CREATE INDEX IF NOT EXISTS leads_company_idx ON leads (company_id);
-- Handoff queue (partial: only rows needing a human stay tiny/hot).
CREATE INDEX IF NOT EXISTS leads_needs_human_idx ON leads (created_at DESC) WHERE needs_human = true;
-- Spec partial-unique "one lead per conversation" (coexists with demo's leads_conversation_id_unique).
CREATE UNIQUE INDEX IF NOT EXISTS leads_conversation_uq
  ON leads (conversation_id) WHERE conversation_id IS NOT NULL;
