package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/angrosist/demo/internal/ports"
	"github.com/angrosist/demo/internal/usecases"
)

// testAdminID is a valid UUID standing in for the admin user that triggered the
// erasure (the proof audit row's actor_id is a UUID column).
const testAdminID = "11111111-2222-3333-4444-555555555555"

// randSuffix returns a short random hex string to keep unique columns
// (companies.reg_no, contacts.email) collision-free across reruns.
func randSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

// recordingStore is an in-memory ports.FileStore that records deletions so the
// erasure test can assert the blobs behind the documents were removed.
type recordingStore struct {
	deleted map[string]bool
}

func newRecordingStore() *recordingStore { return &recordingStore{deleted: map[string]bool{}} }

func (s *recordingStore) Put(context.Context, string, string, io.Reader) error { return nil }
func (s *recordingStore) URL(string) string                                    { return "" }
func (s *recordingStore) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

// Delete records the key as deleted (idempotent).
func (s *recordingStore) Delete(_ context.Context, key string) error {
	s.deleted[key] = true
	return nil
}

var _ ports.FileStore = (*recordingStore)(nil)

// erasureGraph holds the ids created for one full personal graph so assertions can
// reference them.
type erasureGraph struct {
	companyID      string
	contactID      string
	consentID      string
	conversationID string
	leadID         string
	sourcingID     string
	listingID      string
	docConvKey     string
	docListingKey  string
	auditContactID int64
	auditLeadID    int64
	auditOtherID   int64
}

// buildErasureGraph inserts a company (public), a contact (+consent), a
// conversation (+messages), a lead (+sourcing_request + a listing), documents
// owned by the conversation and the listing, and several activity_logs rows (some
// referencing the contact/lead, one unrelated). It returns the created ids and a
// cleanup func that removes the company (the only row that should survive erasure)
// plus any audit rows.
func buildErasureGraph(t *testing.T, ctx context.Context) (erasureGraph, func()) {
	t.Helper()
	pool := GetPool()
	var g erasureGraph

	// Company (public data — must survive erasure). The demo schema keeps cui
	// NOT NULL alongside the canonical reg_no, so set both.
	reg := "ERZ" + randSuffix(t)
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies (country, reg_no, cui, name) VALUES ('RO', $1, $1, 'ERASURE TEST SRL')
		RETURNING id`, reg).Scan(&g.companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	// Contact (personal — root of the cascade).
	if err := pool.QueryRow(ctx, `
		INSERT INTO contacts (company_id, name, phone, email)
		VALUES ($1, 'Test Person', '0712000000', $2)
		RETURNING id`, g.companyID, "erz-"+randSuffix(t)+"@example.com").Scan(&g.contactID); err != nil {
		t.Fatalf("insert contact: %v", err)
	}

	// Consent (+ active pointer).
	if err := pool.QueryRow(ctx, `
		INSERT INTO consents (contact_id, text_version, channel, ip)
		VALUES ($1, 'v1', 'web', '203.0.113.9') RETURNING id`, g.contactID).Scan(&g.consentID); err != nil {
		t.Fatalf("insert consent: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET consent_id = $2 WHERE id = $1`, g.contactID, g.consentID); err != nil {
		t.Fatalf("set consent pointer: %v", err)
	}

	// Conversation (+ messages).
	if err := pool.QueryRow(ctx, `
		INSERT INTO conversations (channel, state, extracted, contact_id)
		VALUES ('web', 'confirmed', '{}', $1) RETURNING id`, g.contactID).Scan(&g.conversationID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	for _, content := range []string{"salut", "buna ziua"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO messages (conversation_id, role, content) VALUES ($1, 'user', $2)`,
			g.conversationID, content); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	}

	// Lead (+ sourcing_request + listing).
	if err := pool.QueryRow(ctx, `
		INSERT INTO leads (conversation_id, contact_id, company_id, vertical, intent, status)
		VALUES ($1, $2, $3, 'angrosist', 'buy', 'new') RETURNING id`,
		g.conversationID, g.contactID, g.companyID).Scan(&g.leadID); err != nil {
		t.Fatalf("insert lead: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO sourcing_requests (lead_id, product, product_name, quantity, unit)
		VALUES ($1, 'mere', 'mere', 1000, 'kg') RETURNING id`, g.leadID).Scan(&g.sourcingID); err != nil {
		t.Fatalf("insert sourcing request: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO listings (lead_id, company_id, stock_type, status)
		VALUES ($1, $2, 'overstock', 'active') RETURNING id`, g.leadID, g.companyID).Scan(&g.listingID); err != nil {
		t.Fatalf("insert listing: %v", err)
	}

	// Documents: a product_list on the conversation + a photo on the listing.
	g.docConvKey = "conversation/" + g.conversationID + "/list.csv"
	g.docListingKey = "listing/" + g.listingID + "/photo.jpg"
	if _, err := pool.Exec(ctx, `
		INSERT INTO documents (owner_type, owner_id, kind, gcs_key) VALUES
		('conversation', $1, 'product_list', $2),
		('listing', $3, 'photo', $4)`,
		g.conversationID, g.docConvKey, g.listingID, g.docListingKey); err != nil {
		t.Fatalf("insert documents: %v", err)
	}

	// Activity logs: one for the contact (PII in meta), one for the lead (PII in
	// meta), and one UNRELATED row that must NOT be redacted.
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_logs (actor_type, action, entity_type, entity_id, meta)
		VALUES ('agent', 'consent.captured', 'contact', $1, $2) RETURNING id`,
		g.contactID, `{"ip":"203.0.113.9","email":"erz@example.com","text_version":"v1"}`).Scan(&g.auditContactID); err != nil {
		t.Fatalf("insert audit (contact): %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_logs (actor_type, action, entity_type, entity_id, meta)
		VALUES ('agent', 'lead.created', 'lead', $1, $2) RETURNING id`,
		g.leadID, `{"ip":"203.0.113.9","phone":"0712000000","vertical":"angrosist"}`).Scan(&g.auditLeadID); err != nil {
		t.Fatalf("insert audit (lead): %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_logs (actor_type, action, entity_type, entity_id, meta)
		VALUES ('system', 'company.verified', 'company', $1, $2) RETURNING id`,
		g.companyID, `{"cui":"123"}`).Scan(&g.auditOtherID); err != nil {
		t.Fatalf("insert audit (company): %v", err)
	}

	cleanup := func() {
		c := context.Background()
		// The company + its company-scoped audit row survive erasure; remove them so
		// reruns stay clean. (Contact/graph are already gone after a successful test.)
		_, _ = GetPool().Exec(c, `DELETE FROM activity_logs WHERE id = ANY($1::bigint[])`,
			[]int64{g.auditContactID, g.auditLeadID, g.auditOtherID})
		_, _ = GetPool().Exec(c, `DELETE FROM companies WHERE id = $1`, g.companyID)
		// Defensive: if the test failed mid-way, drop the contact graph too.
		_, _ = GetPool().Exec(c, `DELETE FROM contacts WHERE id = $1`, g.contactID)
	}
	return g, cleanup
}

// TestErasure_FullCascade builds a complete personal graph, runs the GDPR erasure
// use-case end-to-end (DB cascade + blob deletion + proof audit row), and asserts:
// the personal rows are gone, the company survives, the audit trail is redacted
// (not deleted) with PII stripped, the unrelated audit row is untouched, the blobs
// are deleted, a gdpr.erasure proof row exists, and the report counts are correct.
func TestErasure_FullCascade(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	g, cleanup := buildErasureGraph(t, ctx)
	defer cleanup()

	store := newRecordingStore()
	erasureSvc := usecases.NewErasureService(
		NewErasureRepo(), NewContactRepo(), store, NewActivityLogRepo())

	report, err := erasureSvc.EraseByContactID(ctx, testAdminID, g.contactID)
	if err != nil {
		t.Fatalf("EraseByContactID: %v", err)
	}

	// Report counts.
	if report.LeadsDeleted != 1 || report.ConversationsDeleted != 1 || report.MessagesDeleted != 2 ||
		report.SourcingRequestsDeleted != 1 || report.ListingsDeleted != 1 || report.ConsentsDeleted != 1 ||
		report.DocumentsDeleted != 2 || report.BlobsDeleted != 2 || report.AuditRowsRedacted != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}

	// Personal rows are GONE.
	assertGone(t, ctx, `SELECT count(*) FROM contacts WHERE id = $1`, g.contactID)
	assertGone(t, ctx, `SELECT count(*) FROM consents WHERE id = $1`, g.consentID)
	assertGone(t, ctx, `SELECT count(*) FROM conversations WHERE id = $1`, g.conversationID)
	assertGone(t, ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, g.conversationID)
	assertGone(t, ctx, `SELECT count(*) FROM leads WHERE id = $1`, g.leadID)
	assertGone(t, ctx, `SELECT count(*) FROM sourcing_requests WHERE id = $1`, g.sourcingID)
	assertGone(t, ctx, `SELECT count(*) FROM listings WHERE id = $1`, g.listingID)
	assertGone(t, ctx, `SELECT count(*) FROM documents WHERE owner_id IN ($1, $2)`, g.conversationID, g.listingID)

	// Company (public data) SURVIVES.
	if n := scalar(t, ctx, `SELECT count(*) FROM companies WHERE id = $1`, g.companyID); n != 1 {
		t.Fatalf("company must survive erasure, count = %d", n)
	}

	// Blobs deleted via the FileStore.
	if !store.deleted[g.docConvKey] || !store.deleted[g.docListingKey] {
		t.Fatalf("expected both blobs deleted, got %+v", store.deleted)
	}

	// Audit rows for the contact + lead are REDACTED (present, PII stripped), NOT deleted.
	assertRedacted(t, ctx, g.auditContactID, []string{"ip", "email"})
	assertRedacted(t, ctx, g.auditLeadID, []string{"ip", "phone"})

	// The unrelated company audit row is UNTOUCHED.
	var otherRedacted bool
	if err := GetPool().QueryRow(ctx, `SELECT redacted FROM activity_logs WHERE id = $1`, g.auditOtherID).Scan(&otherRedacted); err != nil {
		t.Fatalf("read unrelated audit row: %v", err)
	}
	if otherRedacted {
		t.Fatal("unrelated company audit row must NOT be redacted")
	}

	// A gdpr.erasure proof row exists for the contact (counts only, no PII).
	var proofMeta map[string]any
	var proofCount int
	row := GetPool().QueryRow(ctx, `
		SELECT count(*) FROM activity_logs
		WHERE action = 'gdpr.erasure' AND entity_type = 'contact' AND entity_id = $1`, g.contactID)
	if err := row.Scan(&proofCount); err != nil {
		t.Fatalf("read proof row count: %v", err)
	}
	if proofCount != 1 {
		t.Fatalf("expected exactly 1 gdpr.erasure proof row, got %d", proofCount)
	}
	if err := GetPool().QueryRow(ctx, `
		SELECT meta FROM activity_logs
		WHERE action = 'gdpr.erasure' AND entity_id = $1 LIMIT 1`, g.contactID).Scan(&proofMeta); err != nil {
		t.Fatalf("read proof meta: %v", err)
	}
	for _, piiKey := range []string{"ip", "email", "phone", "name"} {
		if _, ok := proofMeta[piiKey]; ok {
			t.Fatalf("gdpr.erasure proof meta must not contain PII key %q: %+v", piiKey, proofMeta)
		}
	}

	// Cleanup the proof + any company-scoped audit (added by this test run).
	_, _ = GetPool().Exec(ctx, `DELETE FROM activity_logs WHERE action='gdpr.erasure' AND entity_id=$1`, g.contactID)
}

// TestErasure_ByEmail resolves the same contact via email and erases it.
func TestErasure_ByEmail(t *testing.T) {
	requireDB(t)
	ctx := context.Background()

	// Minimal graph: a contact carrying a known email + a consent.
	pool := GetPool()
	email := "byemail-" + randSuffix(t) + "@example.com"
	var contactID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO contacts (name, email) VALUES ('Email Person', $1) RETURNING id`, email).Scan(&contactID); err != nil {
		t.Fatalf("insert contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO consents (contact_id, text_version, channel) VALUES ($1, 'v1', 'web')`, contactID); err != nil {
		t.Fatalf("insert consent: %v", err)
	}

	svc := usecases.NewErasureService(NewErasureRepo(), NewContactRepo(), newRecordingStore(), NewActivityLogRepo())

	report, err := svc.EraseByEmail(ctx, testAdminID, email)
	if err != nil {
		t.Fatalf("EraseByEmail: %v", err)
	}
	if report.ContactID != contactID || report.ConsentsDeleted != 1 {
		t.Fatalf("unexpected report: %+v (want contact=%s)", report, contactID)
	}
	assertGone(t, ctx, `SELECT count(*) FROM contacts WHERE id = $1`, contactID)

	_, _ = pool.Exec(ctx, `DELETE FROM activity_logs WHERE action='gdpr.erasure' AND entity_id=$1`, contactID)
}

// TestErasure_UnknownContact returns ErrNotFound.
func TestErasure_UnknownContact(t *testing.T) {
	requireDB(t)
	ctx := context.Background()
	svc := usecases.NewErasureService(NewErasureRepo(), NewContactRepo(), newRecordingStore(), NewActivityLogRepo())
	_, err := svc.EraseByContactID(ctx, testAdminID, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected an error for unknown contact")
	}
	if !errorsIsNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- assertion helpers -------------------------------------------------------

func assertGone(t *testing.T, ctx context.Context, sql string, args ...any) {
	t.Helper()
	if n := scalar(t, ctx, sql, args...); n != 0 {
		t.Fatalf("expected 0 rows for %q, got %d", sql, n)
	}
}

func scalar(t *testing.T, ctx context.Context, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := GetPool().QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("scalar query %q: %v", sql, err)
	}
	return n
}

func assertRedacted(t *testing.T, ctx context.Context, id int64, strippedKeys []string) {
	t.Helper()
	var redacted bool
	var meta map[string]any
	if err := GetPool().QueryRow(ctx, `SELECT redacted, meta FROM activity_logs WHERE id = $1`, id).Scan(&redacted, &meta); err != nil {
		t.Fatalf("read audit row %d: %v", id, err)
	}
	if !redacted {
		t.Fatalf("audit row %d must be redacted=true", id)
	}
	for _, k := range strippedKeys {
		if _, ok := meta[k]; ok {
			t.Fatalf("audit row %d meta must not contain PII key %q after redaction: %+v", id, k, meta)
		}
	}
}

func errorsIsNotFound(err error) bool {
	return err != nil && errors.Is(err, ports.ErrNotFound)
}
