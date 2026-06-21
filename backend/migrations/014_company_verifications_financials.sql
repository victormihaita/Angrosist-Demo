-- 014_company_verifications_financials.sql
-- Concern: nullable verification + financials sidecars for companies (§3.2, §3.3, invariant #3).
-- Both are absent for foreign / unverified companies -> they are separate tables, not columns.
-- GDPR: both CASCADE on companies delete (meaningless without the company) -- §4.1.
-- Forward-only/additive: new tables only.
-- Rollback note: DROP TABLE company_financials, company_verifications;

CREATE TABLE IF NOT EXISTS company_verifications (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id     UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,   -- CASCADE: dies with company
  source         TEXT NOT NULL DEFAULT 'demoanaf',   -- doc'd: 'demoanaf' (extensible: 'vies', ...)
  vat_status     TEXT,
  administrators JSONB,                                -- array of {name, role} from DemoANAF
  raw            JSONB,                                -- full DemoANAF /company/:cui payload (cache)
  checked_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query: "latest verification for company X" -> composite, newest first.
CREATE INDEX IF NOT EXISTS company_verifications_company_checked_idx
  ON company_verifications (company_id, checked_at DESC);
-- Note: GIN on `raw` intentionally NOT created (read by company_id; write-cost). Enable on demand.

CREATE TABLE IF NOT EXISTS company_financials (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,      -- CASCADE: dies with company
  year        INT  NOT NULL CHECK (year > 1990 AND year < 2100),            -- coarse, stable invariant only
  turnover    NUMERIC(16,2),
  raw         JSONB,                                  -- DemoANAF /financials payload
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (company_id, year)                           -- one record per company per year (idempotent upsert)
);

-- Query: last N years of turnover for a company -> (company_id, year DESC).
CREATE INDEX IF NOT EXISTS company_financials_company_year_idx
  ON company_financials (company_id, year DESC);
