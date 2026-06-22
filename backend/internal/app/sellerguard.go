package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// sellerConversationGuard adapts the ConversationRepo port to the narrow
// uploadhttp.SellerConversationGuard seam the public photo endpoint depends on.
// It keeps the handler free of any repository dependency while answering the one
// question the endpoint needs: "is this an existing PalletClearance seller
// conversation?". It implements uploadhttp.SellerConversationGuard.
type sellerConversationGuard struct {
	conv ports.ConversationRepo
}

// IsSellerConversation returns (true, nil) for an existing palletclearance/sell
// conversation, (false, nil) for an existing conversation that is not a seller
// conversation, and (false, ports.ErrNotFound) when no such conversation exists.
func (g sellerConversationGuard) IsSellerConversation(ctx context.Context, id string) (bool, error) {
	c, err := g.conv.GetByID(ctx, id)
	if err != nil {
		// Normalize an unknown conversation to ErrNotFound so the handler maps it to
		// 404. The Postgres adapter returns pgx.ErrNoRows for a missing row; any
		// other error is a genuine infrastructure failure surfaced as-is (mapped to
		// 500 by the handler).
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ports.ErrNotFound) {
			return false, ports.ErrNotFound
		}
		return false, err
	}
	if c == nil {
		return false, ports.ErrNotFound
	}
	return c.Vertical == domain.VerticalPalletClearance && c.Intent == domain.IntentSell, nil
}
