package postgres

import (
	"context"
	"testing"
)

// TestSetBotActiveAndHumanFlags asserts the handoff mutations flip the columns
// migrations 016/018 added: conversations.bot_active and leads.needs_human /
// leads.bot_active. It runs against the scratch cluster (skipped without
// DATABASE_URL).
func TestSetBotActiveAndHumanFlags(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	pool := GetPool()

	convRepo := NewConversationRepo()
	leadRepo := NewLeadRepo()

	convID := newConversation(t)

	// A fresh conversation defaults bot_active=true.
	conv, err := convRepo.GetByID(ctx, convID)
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if !conv.BotActive {
		t.Fatal("expected new conversation bot_active=true")
	}

	// Create a lead for the conversation so the flag update has a target.
	var leadID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO leads (conversation_id, status, vertical, intent)
		VALUES ($1::uuid, 'new', 'angrosist', 'buy') RETURNING id::text
	`, convID).Scan(&leadID); err != nil {
		t.Fatalf("insert lead: %v", err)
	}

	// Handoff: mute the conversation + flip the lead flags.
	if err := convRepo.SetBotActive(ctx, convID, false); err != nil {
		t.Fatalf("set bot inactive: %v", err)
	}
	if err := leadRepo.SetHumanFlagsByConversation(ctx, convID, true, false); err != nil {
		t.Fatalf("set human flags: %v", err)
	}

	conv, err = convRepo.GetByID(ctx, convID)
	if err != nil {
		t.Fatalf("reload conversation: %v", err)
	}
	if conv.BotActive {
		t.Fatal("expected bot_active=false after handoff")
	}

	var needsHuman, botActive bool
	if err := pool.QueryRow(ctx,
		`SELECT needs_human, bot_active FROM leads WHERE id = $1::uuid`, leadID,
	).Scan(&needsHuman, &botActive); err != nil {
		t.Fatalf("read lead flags: %v", err)
	}
	if !needsHuman || botActive {
		t.Fatalf("expected lead needs_human=true bot_active=false, got needs_human=%v bot_active=%v", needsHuman, botActive)
	}

	// SetBotActive on a missing conversation reports ErrNotFound.
	if err := convRepo.SetBotActive(ctx, "00000000-0000-0000-0000-000000000000", false); err == nil {
		t.Fatal("expected ErrNotFound for missing conversation")
	}

	// SetHumanFlagsByConversation is a no-op (nil) when no lead exists.
	otherConv := newConversation(t)
	if err := leadRepo.SetHumanFlagsByConversation(ctx, otherConv, true, false); err != nil {
		t.Fatalf("set human flags with no lead should be a no-op: %v", err)
	}

	// Cleanup.
	_, _ = pool.Exec(ctx, `DELETE FROM leads WHERE conversation_id = $1::uuid`, convID)
	_, _ = pool.Exec(ctx, `DELETE FROM conversations WHERE id = ANY($1::uuid[])`,
		[]string{convID, otherConv})
}
