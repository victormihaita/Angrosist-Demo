-- 013_companies_enrich.sql
-- Concern: bring demo `companies` to the Phase-1 target (§3.1) ADDITIVELY (invariant #1, #5).
-- The demo created: cui TEXT UNIQUE, name, address, county, is_active, raw_data, verified_at, created_at.
-- We ADD the canonical dedup key (country, reg_no), roles[], vat_status, caen, updated_at.
-- We KEEP `cui` (legacy alias) and its UNIQUE constraint -- do NOT drop (forward-only/additive).
-- Backfill: country='RO', reg_no = cui for existing rows, then enforce NOT NULL on the canonical key.
-- Rollback note: DROP INDEX companies_country_reg_no_uq, companies_roles_gin, companies_name_trgm;
--                ALTER TABLE companies DROP COLUMN reg_no, country, roles, vat_status, caen, updated_at;

-- 1) Add new columns nullable / defaulted (additive, no reshape of existing columns).
ALTER TABLE companies ADD COLUMN IF NOT EXISTS country     TEXT;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS reg_no      TEXT;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS vat_status  TEXT;                  -- 'active'|'inactive'|'not_registered'|'unknown'|NULL
ALTER TABLE companies ADD COLUMN IF NOT EXISTS caen        TEXT;                  -- RO activity code (nullable for foreign)
ALTER TABLE companies ADD COLUMN IF NOT EXISTS roles       TEXT[] NOT NULL DEFAULT '{}';  -- doc'd values; seeds P2 provider directory
ALTER TABLE companies ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT now();

-- 2) Backfill the canonical dedup key from existing demo data.
UPDATE companies SET country = 'RO' WHERE country IS NULL;
UPDATE companies SET reg_no  = cui  WHERE reg_no IS NULL AND cui IS NOT NULL;

-- 3) Defaults + NOT NULL now that data is backfilled (country generalizes beyond RO).
ALTER TABLE companies ALTER COLUMN country SET DEFAULT 'RO';
ALTER TABLE companies ALTER COLUMN country SET NOT NULL;
-- reg_no NOT NULL: safe because every demo row had a UNIQUE cui that we copied across.
ALTER TABLE companies ALTER COLUMN reg_no SET NOT NULL;

-- 4) INVARIANT #1 dedup key. Generalizes beyond Romanian CUI (country + reg_no).
--    Query: company lookup-on-verify and de-duplication.
CREATE UNIQUE INDEX IF NOT EXISTS companies_country_reg_no_uq ON companies (country, reg_no);

-- 5) Provider directory / role filters (P2 matching seed): roles @> ARRAY['distributor'] etc.
CREATE INDEX IF NOT EXISTS companies_roles_gin ON companies USING gin (roles);

-- 6) B2B directory dashboard name search (trigram; requires pg_trgm from 009).
CREATE INDEX IF NOT EXISTS companies_name_trgm ON companies USING gin (name gin_trgm_ops);
