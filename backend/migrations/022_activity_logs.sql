-- 022_activity_logs.sql
-- Concern: first-class append-only audit log (§3.13, invariant #7, NFR-3).
-- Polymorphic subject (entity_type/entity_id) + polymorphic actor (actor_type/actor_id).
-- GDPR: audit must SURVIVE erasure (FR-9.8 anti-circumvention). On contact erasure these rows are
-- ANONYMIZED/REDACTED in app code (redacted=true, PII stripped from meta) -- NEVER deleted (§4.1, §4.2).
-- Forward-only/additive: new table only.
-- Rollback note: DROP TABLE activity_logs;

CREATE TABLE IF NOT EXISTS activity_logs (
  id          BIGSERIAL PRIMARY KEY,                 -- high-volume append-only
  actor_type  TEXT NOT NULL,                         -- doc'd: 'agent'|'staff'|'provider'|'system'
  actor_id    UUID,                                  -- nullable for 'system'/'agent'
  action      TEXT NOT NULL,                         -- e.g. 'lead.created','company.verified','gdpr.erasure'
  entity_type TEXT,                                  -- polymorphic subject: 'lead'|'company'|'offer'|...
  entity_id   UUID,
  meta        JSONB NOT NULL DEFAULT '{}',           -- structured payload (request ids, diffs, ip, ...)
  redacted    BOOLEAN NOT NULL DEFAULT false,        -- GDPR: true once source personal data erased
  at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query: audit trail for an entity, newest first.
CREATE INDEX IF NOT EXISTS activity_logs_entity_idx ON activity_logs (entity_type, entity_id, at DESC);
-- Query: per-actor audit, newest first.
CREATE INDEX IF NOT EXISTS activity_logs_actor_idx ON activity_logs (actor_type, actor_id, at DESC);
-- Query: global recent activity feed.
CREATE INDEX IF NOT EXISTS activity_logs_at_idx ON activity_logs (at DESC);
-- Query: search inside the payload (containment) -- jsonb_path_ops: smaller/faster for @>.
CREATE INDEX IF NOT EXISTS activity_logs_meta_gin ON activity_logs USING gin (meta jsonb_path_ops);
