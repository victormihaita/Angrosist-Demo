-- 025_leads_offer_tracking.sql
-- Concern: manual offer tracking on the lead (M3 Epic 3.3, API §8.3 PATCH offer).
-- The pipeline STATE lives in leads.status (FK lead_statuses, 010) -- not duplicated here.
-- We ADD only the two value-bearing fields a consultant tracks against an offer:
--   offer_value -- the quoted/negotiated amount (>= 0; nullable until an offer exists)
--   offer_note  -- a free-text consultant note (<= 2000 chars enforced in app)
-- Both are nullable; existing leads keep working untouched.
-- Forward-only/additive: two nullable columns, no reshape.
-- Rollback note: ALTER TABLE leads DROP COLUMN offer_value, DROP COLUMN offer_note;

ALTER TABLE leads ADD COLUMN IF NOT EXISTS offer_value NUMERIC(14,2)
  CHECK (offer_value IS NULL OR offer_value >= 0);            -- quoted amount (>= 0)
ALTER TABLE leads ADD COLUMN IF NOT EXISTS offer_note  TEXT;  -- consultant note (free text)
