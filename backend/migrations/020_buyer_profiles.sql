-- 020_buyer_profiles.sql
-- Concern: standing demand for the PalletClearance buyer feed (§3.9).
-- GDPR: company_id CASCADE (profile belongs to the company) -- §4.1.
-- vertical FK -> verticals(code) (extensible enum).
-- Forward-only/additive: new table only.
-- Rollback note: DROP TABLE buyer_profiles;

CREATE TABLE IF NOT EXISTS buyer_profiles (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id     UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,   -- profile belongs to company
  vertical       TEXT NOT NULL REFERENCES verticals(code),                   -- extensible enum
  categories     UUID[] NOT NULL DEFAULT '{}',        -- category ids of interest (matching)
  countries      TEXT[] NOT NULL DEFAULT '{}',        -- ISO codes of interest
  near_expiry_ok BOOLEAN NOT NULL DEFAULT false,
  subscribed     BOOLEAN NOT NULL DEFAULT true,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query: profiles for a company.
CREATE INDEX IF NOT EXISTS buyer_profiles_company_idx ON buyer_profiles (company_id);
-- Query: when a listing appears, find buyers whose categories overlap (GIN @>/&&).
CREATE INDEX IF NOT EXISTS buyer_profiles_categories_gin ON buyer_profiles USING gin (categories);
-- Query: ... whose countries overlap (GIN @>/&&).
CREATE INDEX IF NOT EXISTS buyer_profiles_countries_gin ON buyer_profiles USING gin (countries);
-- Query: only notify subscribed buyers (partial keeps the feed scan tiny).
CREATE INDEX IF NOT EXISTS buyer_profiles_subscribed_idx ON buyer_profiles (subscribed) WHERE subscribed = true;
