package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ActivityLogRepo is the Postgres adapter for the append-only audit log
// (migration 022). Rows are never updated or deleted here; GDPR erasure
// anonymizes them elsewhere. All statements are parameterized.
type ActivityLogRepo struct{}

// NewActivityLogRepo constructs the Postgres-backed activity log repository.
func NewActivityLogRepo() *ActivityLogRepo { return &ActivityLogRepo{} }

// Append writes one audit row. actor_id/entity_id empty strings map to SQL NULL;
// meta is serialized to JSONB (defaults to '{}' when nil/empty).
func (r *ActivityLogRepo) Append(ctx context.Context, e domain.ActivityLog) error {
	meta := []byte("{}")
	if len(e.Meta) > 0 {
		b, err := json.Marshal(e.Meta)
		if err != nil {
			return fmt.Errorf("marshal activity meta: %w", err)
		}
		meta = b
	}
	_, err := GetPool().Exec(ctx, `
		INSERT INTO activity_logs (actor_type, actor_id, action, entity_type, entity_id, meta)
		VALUES ($1, $2::uuid, $3, $4, $5::uuid, $6)
	`, e.ActorType, nullStr(e.ActorID), e.Action, nullStr(e.EntityType), nullStr(e.EntityID), meta)
	if err != nil {
		return fmt.Errorf("append activity log: %w", err)
	}
	return nil
}

var _ ports.ActivityLogRepo = (*ActivityLogRepo)(nil)
