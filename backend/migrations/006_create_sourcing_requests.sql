CREATE TABLE IF NOT EXISTS sourcing_requests (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lead_id           UUID NOT NULL REFERENCES leads(id),
  product_name      TEXT NOT NULL,
  quantity          NUMERIC,
  unit              TEXT,
  delivery_location TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
