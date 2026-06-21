-- 017_sourcing_requests_enrich.sql
-- Concern: enrich the demo Angrosist typed request (§3.7). One sourcing_request per lead.
-- Demo: id, lead_id (NOT NULL, no UNIQUE), product_name (NOT NULL), quantity NUMERIC, unit,
--        delivery_location, created_at.
-- We ADD canonical `product` (keep demo `product_name`), category_id, recurring, deadline, budget.
-- We ADD the one-per-lead UNIQUE on lead_id (spec) and CASCADE on lead deletion (§4.1).
-- Forward-only/additive: keep demo columns; add canonical alongside; backfill product=product_name.
-- Rollback note: drop added columns/constraints/index; revert lead_id FK to NO ACTION.

-- 1) Additive columns.
ALTER TABLE sourcing_requests ADD COLUMN IF NOT EXISTS product     TEXT;                  -- canonical (demo: product_name)
ALTER TABLE sourcing_requests ADD COLUMN IF NOT EXISTS category_id UUID;
ALTER TABLE sourcing_requests ADD COLUMN IF NOT EXISTS recurring   BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE sourcing_requests ADD COLUMN IF NOT EXISTS deadline    DATE;
ALTER TABLE sourcing_requests ADD COLUMN IF NOT EXISTS budget      NUMERIC(14,2);

-- 2) Backfill canonical product from demo product_name.
UPDATE sourcing_requests SET product = product_name WHERE product IS NULL;

-- 3) category_id FK -> categories, SET NULL (keep request if category removed).
ALTER TABLE sourcing_requests DROP CONSTRAINT IF EXISTS sourcing_requests_category_id_fkey;
ALTER TABLE sourcing_requests ADD CONSTRAINT sourcing_requests_category_id_fkey
  FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL;

-- 4) One typed request per lead + GDPR CASCADE on lead erasure (§3.7, §4.1).
ALTER TABLE sourcing_requests DROP CONSTRAINT IF EXISTS sourcing_requests_lead_id_unique;
ALTER TABLE sourcing_requests ADD CONSTRAINT sourcing_requests_lead_id_unique UNIQUE (lead_id);
ALTER TABLE sourcing_requests DROP CONSTRAINT IF EXISTS sourcing_requests_lead_id_fkey;
ALTER TABLE sourcing_requests ADD CONSTRAINT sourcing_requests_lead_id_fkey
  FOREIGN KEY (lead_id) REFERENCES leads(id) ON DELETE CASCADE;  -- typed request dies with its lead

-- 5) Query: filter sourcing requests by category. (lead_id already indexed by UNIQUE.)
CREATE INDEX IF NOT EXISTS sourcing_requests_category_idx ON sourcing_requests (category_id);
