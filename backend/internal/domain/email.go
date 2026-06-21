package domain

// EmailMessage is the vendor-neutral request the agent/use-cases hand to the
// Mailer port. The adapter owns template rendering and provider delivery, so the
// inner layers never construct subject/body strings or know which provider sends
// the mail. Template + Locale select the rendered body; Data carries the template
// variables.
//
// Keeping the message at the (Template, Locale, Data) level — rather than a
// fully-rendered subject/body — is what lets us swap an SMTP adapter for an
// API-based provider (or change the copy) without touching any call site.
type EmailMessage struct {
	// To is the list of recipient addresses. Must be non-empty for a real send;
	// callers skip the send entirely when there is no recipient (e.g. a prospect
	// with no email on file).
	To []string
	// Template names the body to render, e.g. EmailLeadConfirmation,
	// EmailStaffLeadNotification, EmailHandoffNotification.
	Template string
	// Locale selects the language variant ("ro" | "en"). The adapter falls back to
	// the default locale when a variant is missing.
	Locale string
	// Data holds the template variables (company, product, lead id, reason, ...).
	// It must contain no secrets; ids and short business fields only.
	Data map[string]any
	// ReplyTo is an optional Reply-To address (e.g. the staff inbox on a
	// confirmation so the prospect can reply to a human).
	ReplyTo string
}

// Email template names. These are the stable keys both the agent/use-cases and
// the Mailer adapter agree on; the adapter maps each (name, locale) pair to a
// rendered subject/body.
const (
	// EmailLeadConfirmation acknowledges a submitted request to the prospect.
	EmailLeadConfirmation = "lead_confirmation"
	// EmailStaffLeadNotification tells staff a new qualified lead arrived.
	EmailStaffLeadNotification = "staff_lead_notification"
	// EmailHandoffNotification tells staff a conversation needs a human.
	EmailHandoffNotification = "handoff_notification"
)

// Email locale codes.
const (
	LocaleRO = "ro"
	LocaleEN = "en"
)
