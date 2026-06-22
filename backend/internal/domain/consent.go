package domain

import "time"

// Consent channel codes (consents.channel, migration 015). Documented set, not a
// DB enum (DATA_MODEL §2). Web consent is captured with the request IP; WhatsApp
// consent has no HTTP request IP (the IP is then empty).
const (
	ConsentChannelWeb      = "web"
	ConsentChannelWhatsApp = "whatsapp"
)

// DefaultConsentTextVersion is used when CONSENT_TEXT_VERSION is unset, so consent
// capture always records *some* version string (the schema requires it NOT NULL).
const DefaultConsentTextVersion = "v1"

// Consent is one captured consent event for a contact (consents table, migration
// 015, invariant #7 / NFR-3). It is personal data: it carries the IP at capture
// time as proof and CASCADE-deletes with its contact on erasure. The full history
// lives in consents; the contact's *active* consent is pointed at by
// contacts.consent_id (the deferred circular FK).
type Consent struct {
	// ID is the database-assigned UUID (set on Create).
	ID string
	// ContactID is the data subject the consent belongs to (consents.contact_id).
	ContactID string
	// TextVersion is which consent text/version the subject was shown.
	TextVersion string
	// Channel is where consent was captured: "web" | "whatsapp".
	Channel string
	// IP is the client IP captured at consent time (proof). Empty for WhatsApp /
	// when no request IP is available; stored as SQL NULL then.
	IP string
	// GivenAt is when consent was given (defaults to now() in the adapter).
	GivenAt time.Time
	// CreatedAt is the database-assigned row creation timestamp.
	CreatedAt time.Time
}

// ErasureReport is the structured outcome of a right-to-erasure run (SECURITY.md
// §7.4). It reports counts only — no personal data — so it is safe to return to an
// admin and to log. Counts reflect rows actually removed/redacted in the cascade.
type ErasureReport struct {
	// ContactID is the erased data subject's id (pseudonymous after erasure: the
	// row is gone, only the id remains as a reference for the audit trail).
	ContactID string `json:"contact_id"`
	// LeadsDeleted is the number of leads removed (cascaded from the contact).
	LeadsDeleted int `json:"leads_deleted"`
	// ConversationsDeleted is the number of conversations removed.
	ConversationsDeleted int `json:"conversations_deleted"`
	// MessagesDeleted is the number of transcript messages removed.
	MessagesDeleted int `json:"messages_deleted"`
	// SourcingRequestsDeleted / ListingsDeleted are the typed requests removed.
	SourcingRequestsDeleted int `json:"sourcing_requests_deleted"`
	ListingsDeleted         int `json:"listings_deleted"`
	// ConsentsDeleted is the number of consent rows removed.
	ConsentsDeleted int `json:"consents_deleted"`
	// DocumentsDeleted is the number of documents index rows removed (app-code,
	// no DB cascade reaches them).
	DocumentsDeleted int `json:"documents_deleted"`
	// BlobsDeleted is the number of FileStore objects deleted (best-effort).
	BlobsDeleted int `json:"blobs_deleted"`
	// AuditRowsRedacted is the number of activity_logs rows anonymized (PII
	// stripped, redacted=true) — never deleted (FR-9.8 anti-circumvention).
	AuditRowsRedacted int `json:"audit_rows_redacted"`
}
