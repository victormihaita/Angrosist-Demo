-- 026_conversations_vertical_intent.sql
-- Concern: make a conversation VERTICAL- + INTENT-aware (M4 A1, AI_AGENT_SPEC §1/§2).
-- The flow engine selects the per-(vertical,intent) required-field set + system prompt
-- from these columns. Additive: nullable columns + FKs to the extensible-enum lookups
-- (verticals/intents from migration 010). No destructive default -- existing demo rows
-- stay NULL and the application defaults them to angrosist/buy in code, so current
-- widget behavior is preserved without reshaping data (invariant #5, hard rule #5).
-- Forward-only/additive: new columns + indexes only.
-- Rollback note: drop the added columns/indexes and their FK constraints.

ALTER TABLE conversations ADD COLUMN IF NOT EXISTS vertical TEXT;  -- 'angrosist' | 'palletclearance' (NULL => angrosist in code)
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS intent   TEXT;  -- 'buy' | 'sell'                   (NULL => buy in code)

-- FK to the extensible-enum lookups (010). RESTRICT (taxonomy default) -- a bad value
-- can never be written. NULL is allowed (legacy demo rows), so the FK only validates
-- non-null codes.
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_vertical_fk;
ALTER TABLE conversations ADD CONSTRAINT conversations_vertical_fk
  FOREIGN KEY (vertical) REFERENCES verticals(code);
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_intent_fk;
ALTER TABLE conversations ADD CONSTRAINT conversations_intent_fk
  FOREIGN KEY (intent) REFERENCES intents(code);

-- Query: route/segment conversations by vertical+intent (analytics, flow selection).
CREATE INDEX IF NOT EXISTS conversations_vertical_intent_idx ON conversations (vertical, intent);
