CREATE TABLE IF NOT EXISTS contacts (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id UUID REFERENCES companies(id),
  name       TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
