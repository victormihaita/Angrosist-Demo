CREATE TABLE IF NOT EXISTS companies (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cui         TEXT NOT NULL UNIQUE,
  name        TEXT NOT NULL,
  address     TEXT,
  county      TEXT,
  is_active   BOOLEAN NOT NULL DEFAULT true,
  raw_data    JSONB,
  verified_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
