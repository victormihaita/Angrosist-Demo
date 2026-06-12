CREATE TABLE IF NOT EXISTS leads (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  conversation_id UUID NOT NULL REFERENCES conversations(id),
  company_id      UUID REFERENCES companies(id),
  contact_id      UUID REFERENCES contacts(id),
  status          TEXT NOT NULL DEFAULT 'new',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
