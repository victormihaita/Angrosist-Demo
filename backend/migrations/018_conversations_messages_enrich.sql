-- 018_conversations_messages_enrich.sql
-- Concern: enrich the demo channel-transport tables (§3.11) additively.
-- conversations gains contact_id (GDPR cascade path), language, bot_active.
-- messages gains provider_msg_id (WhatsApp idempotency/dedupe, FR-2.4).
-- Forward-only/additive: new columns + indexes only; messages_conversation_created_idx already in 003.
-- Rollback note: drop added columns/indexes.

-- conversations.contact_id -> contacts, CASCADE (conversation is personal context, §4.1).
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS contact_id UUID;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS language   TEXT;                          -- detected language
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS bot_active BOOLEAN NOT NULL DEFAULT true; -- handoff mute

ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_contact_id_fkey;
ALTER TABLE conversations ADD CONSTRAINT conversations_contact_id_fkey
  FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE;  -- §4.1

-- Query: conversations for a contact (GDPR cascade + timeline).
CREATE INDEX IF NOT EXISTS conversations_contact_idx ON conversations (contact_id);

-- messages.provider_msg_id: WhatsApp message id for inbound idempotency.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS provider_msg_id TEXT;

-- Query: dedupe inbound WhatsApp by provider message id (FR-2.4). Partial unique skips web nulls.
CREATE UNIQUE INDEX IF NOT EXISTS messages_provider_msg_uq
  ON messages (provider_msg_id) WHERE provider_msg_id IS NOT NULL;
