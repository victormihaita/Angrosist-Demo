-- 024_users_password_hash.sql
-- Concern: enable staff/admin password login (M3 Epic 3.3, SECURITY §2/§3).
-- The demo `users` table (011) had no credential column. We ADD a nullable
-- `password_hash` (bcrypt) so existing rows (and SSO/OTP-only future users)
-- stay valid without one. Login simply rejects users whose hash is NULL.
-- The hash NEVER contains the plaintext; bcrypt is computed in app code.
-- Forward-only/additive: one nullable column, no reshape of existing data.
-- Rollback note: ALTER TABLE users DROP COLUMN password_hash;

ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;  -- bcrypt; NULL = no password login
