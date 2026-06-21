-- 019_listings.sql
-- Concern: PalletClearance supply/clearance typed request (§3.8). Sibling of the lead (invariant #5).
-- One listing per lead (UNIQUE lead_id), CASCADE on lead erasure (§4.1).
-- Seller photos are mandatory but enforced by the FLOW ENGINE (app layer), not the DB -- documented.
-- GDPR FKs: lead_id CASCADE; company_id SET NULL; category_id SET NULL (§4.1).
-- Forward-only/additive: new table only.
-- Rollback note: DROP TABLE listings;

CREATE TABLE IF NOT EXISTS listings (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id      UUID NOT NULL UNIQUE REFERENCES leads(id) ON DELETE CASCADE,   -- one per lead; dies with lead
  company_id   UUID REFERENCES companies(id) ON DELETE SET NULL,              -- seller company (public)
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

-- Query: inventory dashboard filtered by country + status.
CREATE INDEX IF NOT EXISTS listings_country_status_idx ON listings (country, status);
-- Query: near-expiry feed (FR-3.2) -- partial on active only, soonest-expiring first.
CREATE INDEX IF NOT EXISTS listings_expiry_idx ON listings (expiry) WHERE status = 'active';
-- Query: filter listings by category.
CREATE INDEX IF NOT EXISTS listings_category_idx ON listings (category_id);
-- Query: listings for a seller company.
CREATE INDEX IF NOT EXISTS listings_company_idx ON listings (company_id);
