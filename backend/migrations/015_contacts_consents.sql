-- 015_contacts_consents.sql
-- Concern: enrich demo `contacts` (§3.4) + first-class `consents` (§3.10, invariant #7).
-- contacts is the ROOT of the personal-data graph (GDPR cascade origin, §4).
-- Demo contacts: id, company_id, name, phone, email, created_at. We ADD language, consent_id, updated_at.
-- consents.contact_id CASCADE (consent is personal, dies with contact). contacts.consent_id deferred FK
-- SET NULL (active-consent pointer; history lives in consents). Circular dep resolved per spec §3.10.
-- Also fix contacts.company_id FK -> ON DELETE SET NULL (company is public; contact survives) -- §4.1.
-- Forward-only/additive: new columns + new table + FK behavior corrections (no data reshape).
-- Rollback note: DROP TABLE consents; ALTER TABLE contacts DROP COLUMN language, consent_id, updated_at;

-- 1) Additive contacts columns.
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS language   TEXT NOT NULL DEFAULT 'ro';  -- 'ro'|'en' (FR-2.6)
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS consent_id UUID;                          -- FK added below
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- 2) Personal-data lookup indexes (idempotency / inbound resolution).
CREATE INDEX IF NOT EXISTS contacts_company_idx ON contacts (company_id);
CREATE INDEX IF NOT EXISTS contacts_phone_idx   ON contacts (phone);  -- WhatsApp inbound by sender number
CREATE INDEX IF NOT EXISTS contacts_email_idx   ON contacts (email);

-- 3) Correct contacts.company_id ON DELETE behavior to SET NULL (§4.1).
--    Demo FK (004) has default NO ACTION; re-point only the constraint (data untouched).
ALTER TABLE contacts DROP CONSTRAINT IF EXISTS contacts_company_id_fkey;
ALTER TABLE contacts
  ADD CONSTRAINT contacts_company_id_fkey
  FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE SET NULL;

-- 4) consents: full history per contact; CASCADE on contact erasure (§4.1).
CREATE TABLE IF NOT EXISTS consents (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  contact_id   UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,  -- CASCADE: consent is personal
  text_version TEXT NOT NULL,                        -- which consent text/version was shown
  given_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  channel      TEXT NOT NULL,                        -- doc'd: 'web'|'whatsapp'
  ip           INET,                                 -- captured at consent (proof)
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query: consent history for a contact, newest first.
CREATE INDEX IF NOT EXISTS consents_contact_idx ON consents (contact_id, given_at DESC);

-- 5) Deferred FK: contacts.consent_id -> consents.id (active-consent pointer).
--    SET NULL: erasing a consent row clears the pointer; the contact keeps existing (§3.10).
ALTER TABLE contacts DROP CONSTRAINT IF EXISTS contacts_consent_fk;
ALTER TABLE contacts
  ADD CONSTRAINT contacts_consent_fk
  FOREIGN KEY (consent_id) REFERENCES consents(id) ON DELETE SET NULL;
