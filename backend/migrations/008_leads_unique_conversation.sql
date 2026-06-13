-- Delete sourcing_requests belonging to duplicate leads (keep oldest per conversation).
DELETE FROM sourcing_requests
WHERE lead_id IN (
    SELECT id FROM leads
    WHERE id NOT IN (
        SELECT DISTINCT ON (conversation_id) id
        FROM leads
        ORDER BY conversation_id, created_at ASC
    )
);

-- Remove duplicate leads, keeping the oldest per conversation.
DELETE FROM leads
WHERE id NOT IN (
    SELECT DISTINCT ON (conversation_id) id
    FROM leads
    ORDER BY conversation_id, created_at ASC
);

-- Enforce one lead per conversation going forward.
ALTER TABLE leads
    ADD CONSTRAINT leads_conversation_id_unique UNIQUE (conversation_id);
