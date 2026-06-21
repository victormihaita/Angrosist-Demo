-- 021_documents.sql
-- Concern: polymorphic document index, files always in GCS (§3.12, invariant #6).
-- (owner_type, owner_id) is a SOFT polymorphic FK -- no DB FK can span multiple owner tables.
-- GDPR consequence: there is NO cascade into documents; deletion (DB row + GCS blob behind gcs_key)
-- is an APP-CODE step in the erasure path (§4.2). Documented here so it is never assumed automatic.
-- Forward-only/additive: new table only.
-- Rollback note: DROP TABLE documents;

CREATE TABLE IF NOT EXISTS documents (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type  TEXT NOT NULL,                         -- 'lead'|'listing'|'offer'|'sourcing_request' (doc'd, extensible)
  owner_id    UUID NOT NULL,                         -- soft polymorphic FK (no DB FK across types)
  kind        TEXT NOT NULL,                         -- 'photo'|'product_list'|'offer' (doc'd, extensible)
  gcs_key     TEXT NOT NULL,                         -- INVARIANT #6: files always in GCS, never on disk
  mime        TEXT,
  size_bytes  BIGINT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query: "all documents for this lead/listing/offer".
CREATE INDEX IF NOT EXISTS documents_owner_idx ON documents (owner_type, owner_id);
