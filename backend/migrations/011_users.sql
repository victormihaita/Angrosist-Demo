-- 011_users.sql
-- Concern: staff/admin user table (§3.14). Referenced by leads.assigned_to (added in 016).
-- `role` is a documented TEXT set (NOT an enum type) so P2 roles add with zero DDL:
--   P1: 'staff'|'admin'   P2: 'provider'|'admin_global'|'country_operator'
-- Forward-only/additive: new table only.
-- Rollback note: DROP TABLE users; (only before leads.assigned_to FK exists).

CREATE TABLE IF NOT EXISTS users (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email      TEXT NOT NULL UNIQUE,
  name       TEXT,
  role       TEXT NOT NULL DEFAULT 'staff',          -- doc'd TEXT set; see header
  active     BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- email is already unique-indexed by the UNIQUE constraint (login/lookup by email).
