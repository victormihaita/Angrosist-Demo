package usecases

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

// Retention policy (SECURITY.md §7.3 — documented here; the automated sweep is
// DEFERRED, owner-gated at provisioning).
//
// Data classes and their intended retention:
//   - Raw conversation transcripts (conversations + messages): SHORT retention —
//     they are personal data with little business value once a lead is normalized.
//     Candidate for an automated purge (e.g. 90 days after the conversation's last
//     message) independent of erasure.
//   - Normalized leads + typed requests (leads, sourcing_requests, listings):
//     business-defined retention (the qualified outcome), erased on a data-subject
//     request via the cascade below.
//   - consents: retained as proof for the lifetime of the related personal data;
//     erased with the contact.
//   - companies / company_verifications / company_financials: PUBLIC/business data
//     — RETAINED as the strategic B2B asset; never purged by retention or erasure.
//   - activity_logs: retained for audit/anti-circumvention (FR-9.8); on erasure
//     they are REDACTED (PII stripped), never deleted.
//
// The scheduled purge job is intentionally NOT built here: the Cloud Scheduler
// retention cron is provisioned (and its cadence chosen) by the owner at
// deployment. When it lands it should reuse this package's ports (a new
// RetentionRepo) and follow the same "redact audit, preserve public data" rules as
// the erasure cascade. Until then, the right-to-erasure path (below) is the only
// deletion path and it is admin-triggered, not automatic.

// ErasureService implements the GDPR right-to-erasure use-case (SECURITY.md §7.4).
// It orchestrates ports only — the transactional database cascade is owned by the
// ErasureRepo adapter, blob deletion by the FileStore, and the proof-of-erasure
// audit row by the ActivityLogRepo. No infrastructure or SQL leaks in here.
//
// Order of operations (matching the spec):
//  1. ErasureRepo.Erase runs the DB cascade in one transaction and returns the
//     collected blob keys + a counts report (contact + personal graph deleted,
//     companies preserved, audit rows redacted not deleted).
//  2. After the DB commit, the collected blobs are deleted from the FileStore
//     (best-effort; a failure is logged, not fatal — the personal data in the DB
//     is already gone, and FileStore.Delete is idempotent on retry).
//  3. A final "gdpr.erasure" activity_logs row is written (counts only, no PII).
type ErasureService struct {
	erasure  ports.ErasureRepo
	contacts ports.ContactRepo
	files    ports.FileStore
	activity ports.ActivityLogRepo
}

// NewErasureService wires the erasure use-case. files and activity may be nil in
// reduced wirings (blob deletion / the proof audit row are then skipped, logged);
// the production wiring supplies all.
func NewErasureService(erasure ports.ErasureRepo, contacts ports.ContactRepo, files ports.FileStore, activity ports.ActivityLogRepo) *ErasureService {
	return &ErasureService{erasure: erasure, contacts: contacts, files: files, activity: activity}
}

// EraseByContactID performs the cascade for an explicit contact id. actorID is the
// admin user id that triggered the request (recorded in the proof audit row). It
// returns ports.ErrNotFound when no such contact exists.
func (s *ErasureService) EraseByContactID(ctx context.Context, actorID, contactID string) (domain.ErasureReport, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return domain.ErasureReport{}, ports.ErrNotFound
	}
	return s.erase(ctx, actorID, contactID)
}

// EraseByEmail resolves the contact from an email (convenience for a data-subject
// request that arrives by email) then runs the cascade. It returns
// ports.ErrNotFound when no contact carries the email.
func (s *ErasureService) EraseByEmail(ctx context.Context, actorID, email string) (domain.ErasureReport, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return domain.ErasureReport{}, ports.ErrNotFound
	}
	contactID, err := s.contacts.FindIDByEmail(ctx, email)
	if err != nil {
		return domain.ErasureReport{}, err // includes ports.ErrNotFound
	}
	return s.erase(ctx, actorID, contactID)
}

// erase runs the DB cascade, deletes the returned blobs best-effort, and writes
// the proof audit row.
func (s *ErasureService) erase(ctx context.Context, actorID, contactID string) (domain.ErasureReport, error) {
	report, blobKeys, err := s.erasure.Erase(ctx, contactID)
	if err != nil {
		return domain.ErasureReport{}, fmt.Errorf("erase contact: %w", err)
	}

	// Delete the collected blobs out-of-band (Postgres cannot remove a GCS object;
	// a DB cascade alone would orphan the blob — SECURITY.md §7.4). Best-effort:
	// FileStore.Delete is idempotent, so a transient failure can be retried.
	if s.files != nil {
		for _, key := range blobKeys {
			if derr := s.files.Delete(ctx, key); derr != nil {
				// Orphaned blob (residual N2). Keys are server-generated namespaced
				// paths, not personal data. Record each failure to the audit log so a
				// reconcile/retry can find and re-delete it (FileStore.Delete is
				// idempotent), and count it in the report so the admin caller knows a
				// follow-up is needed — not just a silent log line.
				report.BlobsFailed++
				log.Printf("erasure: delete blob failed (contact=%s): %v", contactID, derr)
				if s.activity != nil {
					_ = s.activity.Append(ctx, domain.ActivityLog{
						ActorType:  "admin",
						ActorID:    actorID,
						Action:     "gdpr.blob_delete_failed",
						EntityType: "document_blob",
						EntityID:   key,
						Meta:       map[string]any{"error": derr.Error()},
					})
				}
				continue
			}
			report.BlobsDeleted++
		}
	}

	// Proof-of-erasure audit row: counts only, references the (now-deleted) contact
	// id pseudonymously. It must itself carry no PII.
	if s.activity != nil {
		if aerr := s.activity.Append(ctx, domain.ActivityLog{
			ActorType:  "admin",
			ActorID:    actorID,
			Action:     "gdpr.erasure",
			EntityType: "contact",
			EntityID:   contactID,
			Meta: map[string]any{
				"leads_deleted":             report.LeadsDeleted,
				"conversations_deleted":     report.ConversationsDeleted,
				"messages_deleted":          report.MessagesDeleted,
				"sourcing_requests_deleted": report.SourcingRequestsDeleted,
				"listings_deleted":          report.ListingsDeleted,
				"consents_deleted":          report.ConsentsDeleted,
				"documents_deleted":         report.DocumentsDeleted,
				"blobs_deleted":             report.BlobsDeleted,
				"blobs_failed":              report.BlobsFailed,
				"audit_rows_redacted":       report.AuditRowsRedacted,
			},
		}); aerr != nil {
			log.Printf("erasure: append proof audit row (contact=%s): %v", contactID, aerr)
		}
	}

	return report, nil
}
