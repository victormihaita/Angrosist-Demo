-- 009_enable_extensions.sql
-- Concern: enable PostgreSQL extensions required by the Phase-1 target schema.
-- pgcrypto       -> gen_random_uuid() (already relied on by demo 001-008; make explicit)
-- pg_trgm        -> trigram GIN index for companies.name B2B directory search (§3.1)
-- Forward-only/additive: extensions only; touches no existing data.
-- Rollback note: DROP EXTENSION IF EXISTS pg_trgm; DROP EXTENSION IF EXISTS pgcrypto;
--                (only if no objects depend on them).

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
