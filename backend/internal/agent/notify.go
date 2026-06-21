package agent

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/angrosist/demo/internal/domain"
)

// maxHandoffSummary bounds the staff-facing summary to keep notification payloads
// (and the audit meta) small and free of pasted transcripts.
const maxHandoffSummary = 600

// toolHandoff executes the handoff_to_human tool (FR-6.1, AI_AGENT_SPEC §5.4). It
// mutes the bot for the conversation (bot_active=false, needs_human=true on the
// conversation and any existing lead), notifies staff by email (best-effort), and
// writes an activity_logs "handoff" row. It always returns {handed_off: true} so
// the model emits a brief closing line and stops.
func (c *Core) toolHandoff(ctx context.Context, conv *domain.Conversation, args map[string]any) (map[string]any, error) {
	reason := strings.TrimSpace(strArg(args["reason"]))
	if !handoffReasons[reason] {
		reason = "other"
	}
	summary := truncate(strings.TrimSpace(strArg(args["summary"])), maxHandoffSummary)

	// Mute the bot for this conversation so the worker short-circuits future turns.
	if err := c.convRepo.SetBotActive(ctx, conv.ID, false); err != nil {
		// Non-fatal: log and continue. The handoff must still notify staff.
		log.Printf("handoff: set bot inactive conversation=%s: %v", conv.ID, err)
	} else {
		conv.BotActive = false
	}

	// Flip needs_human/bot_active on the lead if one already exists (no-op otherwise).
	var leadID string
	if lead, err := c.leadRepo.GetByConversationID(ctx, conv.ID); err == nil && lead != nil {
		leadID = lead.ID
		if err := c.leadRepo.SetHumanFlagsByConversation(ctx, conv.ID, true, false); err != nil {
			log.Printf("handoff: set lead human flags conversation=%s: %v", conv.ID, err)
		}
	}

	locale := c.localeFor(conv)
	c.sendEmail(ctx, domain.EmailMessage{
		To:       c.staffRecipients(),
		Template: domain.EmailHandoffNotification,
		Locale:   locale,
		Data: map[string]any{
			"Reason":         reason,
			"Summary":        summary,
			"ConversationID": conv.ID,
			"LeadID":         leadID,
		},
	}, "handoff")

	c.logActivity(ctx, domain.ActivityLog{
		ActorType:  "agent",
		Action:     "handoff",
		EntityType: "conversation",
		EntityID:   conv.ID,
		Meta: map[string]any{
			"reason":  reason,
			"lead_id": leadID,
		},
	})

	return map[string]any{"handed_off": true}, nil
}

// notifyLeadSubmitted sends the prospect confirmation (when an email is on file)
// and the internal staff notification after a lead is created/updated. It is
// best-effort: every failure is logged (no PII) and never affects the turn or the
// save_lead tool result. It also writes an "email.sent" / "email.skipped" audit
// row capturing recipient counts only.
func (c *Core) notifyLeadSubmitted(ctx context.Context, conv *domain.Conversation, leadID string, company *domain.Company, req *domain.SourcingRequest, phone, email string) {
	if c.mailer == nil {
		return
	}
	locale := c.localeFor(conv)

	companyName := ""
	cui := ""
	if company != nil {
		companyName = company.Name
		cui = company.CUI
	}

	confirmationSent := false
	if strings.TrimSpace(email) != "" {
		c.sendEmail(ctx, domain.EmailMessage{
			To:       []string{email},
			Template: domain.EmailLeadConfirmation,
			Locale:   locale,
			ReplyTo:  c.staffNotify,
			Data: map[string]any{
				"Product":  req.ProductName,
				"Quantity": formatQty(req.Quantity),
				"Unit":     req.Unit,
				"Company":  companyName,
			},
		}, "lead_confirmation")
		confirmationSent = true
	}

	staff := c.staffRecipients()
	staffSent := false
	if len(staff) > 0 {
		c.sendEmail(ctx, domain.EmailMessage{
			To:       staff,
			Template: domain.EmailStaffLeadNotification,
			Locale:   locale,
			Data: map[string]any{
				"Company":          companyName,
				"CUI":              cui,
				"Product":          req.ProductName,
				"Quantity":         formatQty(req.Quantity),
				"Unit":             req.Unit,
				"DeliveryLocation": req.DeliveryLocation,
				"Phone":            phone,
				"Email":            email,
				"LeadID":           leadID,
			},
		}, "staff_lead_notification")
		staffSent = true
	}

	c.logActivity(ctx, domain.ActivityLog{
		ActorType:  "agent",
		Action:     "email.sent",
		EntityType: "lead",
		EntityID:   leadID,
		Meta: map[string]any{
			"confirmation_sent": confirmationSent,
			"staff_notified":    staffSent,
			"recipients":        boolToInt(confirmationSent) + len(staff),
		},
	})
}

// sendEmail invokes the Mailer and logs a failure without aborting. label is the
// non-PII tag used in logs.
func (c *Core) sendEmail(ctx context.Context, msg domain.EmailMessage, label string) {
	if c.mailer == nil || len(msg.To) == 0 {
		return
	}
	if err := c.mailer.Send(ctx, msg); err != nil {
		log.Printf("email send failed template=%s recipients=%d: %v", label, len(msg.To), err)
	}
}

// logActivity appends an audit row, swallowing errors (audit is best-effort and
// must never fail a turn).
func (c *Core) logActivity(ctx context.Context, e domain.ActivityLog) {
	if c.activityLog == nil {
		return
	}
	if err := c.activityLog.Append(ctx, e); err != nil {
		log.Printf("activity log append action=%s: %v", e.Action, err)
	}
}

// staffRecipients returns the configured staff inbox as a recipient list, or nil
// when unset (staff email then disabled).
func (c *Core) staffRecipients() []string {
	if strings.TrimSpace(c.staffNotify) == "" {
		return nil
	}
	return []string{c.staffNotify}
}

// localeFor picks the email locale: the conversation language when known, else
// the configured default.
func (c *Core) localeFor(conv *domain.Conversation) string {
	if conv != nil && strings.TrimSpace(conv.Language) != "" {
		return conv.Language
	}
	if c.defaultLang != "" {
		return c.defaultLang
	}
	return domain.LocaleRO
}

// truncate caps s to n runes (no ellipsis; staff summaries are already short).
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// boolToInt maps true→1, false→0 for the recipient count in audit meta.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// formatQty renders a nullable quantity for templates (empty when nil). Trailing
// zeros are trimmed so "10" prints as "10", not "10.000000".
func formatQty(q *float64) string {
	if q == nil {
		return ""
	}
	s := strconv.FormatFloat(*q, 'f', -1, 64)
	return s
}
