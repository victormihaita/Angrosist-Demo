package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// ErasureRepo is the Postgres half of the GDPR right-to-erasure cascade
// (SECURITY.md §7.4, DATA_MODEL_DDL §4). It owns its transaction internally so no
// pgx handle/tx leaks across the port boundary (hexagonal). All statements are
// parameterized.
type ErasureRepo struct{}

// NewErasureRepo constructs the Postgres-backed erasure repository.
func NewErasureRepo() *ErasureRepo { return &ErasureRepo{} }

// erasurePIIKeys are the meta keys stripped from activity_logs during redaction.
// The audit ROW is preserved (redacted=true) for anti-circumvention/retention
// (FR-9.8); only direct personal identifiers inside meta are removed. Entity ids
// (lead_id, conversation_id) are retained as pseudonymous references.
var erasurePIIKeys = []string{"ip", "phone", "email", "name", "address", "phone_number"}

// Erase runs the documented cascade for contactID in a single transaction:
//
//  1. resolve the contact (ErrNotFound when absent);
//  2. COLLECT every documents.gcs_key reachable from the contact (its
//     conversations, leads, and the leads' listings/sourcing_requests) BEFORE
//     deleting anything — documents has no DB FK, so this is app-owned;
//  3. ANONYMIZE (redacted=true, PII stripped from meta) the activity_logs rows
//     referencing the erased entities — never delete the audit trail;
//  4. DELETE the documents index rows (no FK cascade reaches them);
//  5. DELETE the contact — the FK CASCADE chain removes consents,
//     conversations→messages, leads→(sourcing_requests|listings). companies are
//     NOT deleted (public data; links are SET NULL by the FKs);
//  6. commit; the returned blob keys are deleted out-of-band by the use-case.
//
// Counts are captured BEFORE the contact DELETE (the rows are gone afterward).
func (r *ErasureRepo) Erase(ctx context.Context, contactID string) (domain.ErasureReport, []string, error) {
	report := domain.ErasureReport{ContactID: contactID}

	tx, err := GetPool().Begin(ctx)
	if err != nil {
		return report, nil, fmt.Errorf("erasure: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1) Confirm the contact exists (the data subject root). Lock the row so a
	//    concurrent erasure of the same subject serializes.
	var exists string
	err = tx.QueryRow(ctx, `SELECT id FROM contacts WHERE id = $1::uuid FOR UPDATE`, contactID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return report, nil, ports.ErrNotFound
	}
	if err != nil {
		return report, nil, fmt.Errorf("erasure: load contact: %w", err)
	}

	// Resolve the entity ids in the personal graph (leads + conversations) so we
	// can both count and redact their audit rows.
	leadIDs, err := collectIDs(ctx, tx, `SELECT id FROM leads WHERE contact_id = $1::uuid`, contactID)
	if err != nil {
		return report, nil, fmt.Errorf("erasure: collect leads: %w", err)
	}
	convIDs, err := collectIDs(ctx, tx, `SELECT id FROM conversations WHERE contact_id = $1::uuid`, contactID)
	if err != nil {
		return report, nil, fmt.Errorf("erasure: collect conversations: %w", err)
	}

	// 2) Collect blob keys + document ids reachable from the contact BEFORE delete.
	docKeys, docIDs, err := r.collectDocuments(ctx, tx, contactID)
	if err != nil {
		return report, nil, err
	}
	report.DocumentsDeleted = len(docIDs)

	// Capture counts (these cascade away on the contact DELETE, so count first).
	report.LeadsDeleted = len(leadIDs)
	report.ConversationsDeleted = len(convIDs)
	if report.MessagesDeleted, err = countScalar(ctx, tx, `
		SELECT count(*) FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.contact_id = $1::uuid`, contactID); err != nil {
		return report, nil, fmt.Errorf("erasure: count messages: %w", err)
	}
	if report.SourcingRequestsDeleted, err = countScalar(ctx, tx, `
		SELECT count(*) FROM sourcing_requests s
		JOIN leads l ON l.id = s.lead_id WHERE l.contact_id = $1::uuid`, contactID); err != nil {
		return report, nil, fmt.Errorf("erasure: count sourcing requests: %w", err)
	}
	if report.ListingsDeleted, err = countScalar(ctx, tx, `
		SELECT count(*) FROM listings li
		JOIN leads l ON l.id = li.lead_id WHERE l.contact_id = $1::uuid`, contactID); err != nil {
		return report, nil, fmt.Errorf("erasure: count listings: %w", err)
	}
	if report.ConsentsDeleted, err = countScalar(ctx, tx,
		`SELECT count(*) FROM consents WHERE contact_id = $1::uuid`, contactID); err != nil {
		return report, nil, fmt.Errorf("erasure: count consents: %w", err)
	}

	// 3) Anonymize activity_logs referencing the contact and its leads/conversations.
	//    Preserve the rows (redacted=true), strip PII from meta. Never delete.
	report.AuditRowsRedacted, err = r.redactAuditLogs(ctx, tx, contactID, leadIDs, convIDs)
	if err != nil {
		return report, nil, err
	}

	// 4) Delete the collected document rows (no FK cascade reaches them).
	if len(docIDs) > 0 {
		if _, err := tx.Exec(ctx,
			`DELETE FROM documents WHERE id = ANY($1::uuid[])`, docIDs); err != nil {
			return report, nil, fmt.Errorf("erasure: delete documents: %w", err)
		}
	}

	// 5) Delete the contact — FK CASCADE removes consents, conversations→messages,
	//    leads→typed requests. companies survive (SET NULL on links).
	if _, err := tx.Exec(ctx, `DELETE FROM contacts WHERE id = $1::uuid`, contactID); err != nil {
		return report, nil, fmt.Errorf("erasure: delete contact: %w", err)
	}

	// 6) Commit. Blob deletion happens out-of-band (the use-case) after commit.
	if err := tx.Commit(ctx); err != nil {
		return report, nil, fmt.Errorf("erasure: commit: %w", err)
	}
	return report, docKeys, nil
}

// collectDocuments returns the gcs_key + id of every document owned by an entity
// reachable from the contact: its conversations, its leads, and those leads'
// listings/sourcing_requests. The (owner_type, owner_id) pair is a soft
// polymorphic reference, so this is an explicit query, not a cascade.
func (r *ErasureRepo) collectDocuments(ctx context.Context, tx pgx.Tx, contactID string) (keys, ids []string, err error) {
	rows, err := tx.Query(ctx, `
		SELECT d.gcs_key, d.id
		FROM documents d
		WHERE (d.owner_type, d.owner_id) IN (
			SELECT 'conversation', c.id FROM conversations c WHERE c.contact_id = $1::uuid
			UNION ALL
			SELECT 'lead', l.id FROM leads l WHERE l.contact_id = $1::uuid
			UNION ALL
			SELECT 'listing', li.id FROM listings li
				JOIN leads l ON l.id = li.lead_id WHERE l.contact_id = $1::uuid
			UNION ALL
			SELECT 'sourcing_request', s.id FROM sourcing_requests s
				JOIN leads l ON l.id = s.lead_id WHERE l.contact_id = $1::uuid
		)
	`, contactID)
	if err != nil {
		return nil, nil, fmt.Errorf("erasure: collect documents: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			return nil, nil, fmt.Errorf("erasure: scan document: %w", err)
		}
		keys = append(keys, key)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("erasure: iterate documents: %w", err)
	}
	return keys, ids, nil
}

// redactAuditLogs anonymizes (redacted=true, PII keys removed from meta) every
// activity_logs row whose (entity_type, entity_id) references the erased contact,
// one of its leads, or one of its conversations. The audit chain is preserved.
func (r *ErasureRepo) redactAuditLogs(ctx context.Context, tx pgx.Tx, contactID string, leadIDs, convIDs []string) (int, error) {
	// Strip each PII key from meta (`meta - text[]` removes all listed keys), set
	// redacted=true. entity_type is matched alongside entity_id so we never redact
	// an unrelated row that happens to share a UUID across tables. Empty id slices
	// encode to empty arrays (matching nothing), which is safe.
	tag, err := tx.Exec(ctx, `
		UPDATE activity_logs
		SET redacted = true,
		    meta = meta - $1::text[]
		WHERE
		    (entity_type = 'contact'      AND entity_id = $2::uuid) OR
		    (entity_type = 'lead'         AND entity_id = ANY($3::uuid[])) OR
		    (entity_type = 'conversation' AND entity_id = ANY($4::uuid[]))
	`, erasurePIIKeys, contactID, leadIDs, convIDs)
	if err != nil {
		return 0, fmt.Errorf("erasure: redact activity logs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// collectIDs runs a single-column id query and returns the ids as strings.
func collectIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// countScalar runs a count(*) query and returns the integer result.
func countScalar(ctx context.Context, tx pgx.Tx, sql string, args ...any) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

var _ ports.ErasureRepo = (*ErasureRepo)(nil)
