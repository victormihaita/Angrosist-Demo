-- 023_templates.sql
-- Concern: email/message templates, RO/EN (§3.15). Standalone, no FKs.
-- UNIQUE (code, channel, language): one template per (code, channel, lang).
-- Forward-only/additive: new table only.
-- Rollback note: DROP TABLE templates;

CREATE TABLE IF NOT EXISTS templates (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code       TEXT NOT NULL,                          -- stable key, e.g. 'lead_confirmation'
  channel    TEXT NOT NULL,                          -- 'email'|'whatsapp'
  language   TEXT NOT NULL,                          -- 'ro'|'en'
  subject    TEXT,                                   -- email only
  body       TEXT NOT NULL,
  active     BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (code, channel, language)
);

-- Query: resolve the active template for (code, channel, language).
CREATE INDEX IF NOT EXISTS templates_lookup_idx ON templates (code, channel, language) WHERE active = true;
