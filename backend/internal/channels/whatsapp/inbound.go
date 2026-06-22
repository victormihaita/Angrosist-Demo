package whatsapp

import (
	"encoding/json"
	"fmt"
)

// InboundMessage is the normalized form of a single user WhatsApp text message
// extracted from a Cloud API webhook payload. Status callbacks (delivered/read/
// sent) and non-text message types are not represented — they are ignored by the
// parser.
type InboundMessage struct {
	// From is the sender's phone number (digits, no '+'), as Meta provides it.
	From string
	// ID is the WhatsApp message id (wamid.*), used as the idempotency/dedupe key.
	ID string
	// Text is the message body.
	Text string
}

// webhookEnvelope models the subset of the Cloud API webhook payload we consume:
// entry[].changes[].value.messages[] with from, id, text.body. Everything else
// (contacts, metadata, statuses) is ignored; unknown fields are tolerated.
type webhookEnvelope struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					ID   string `json:"id"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// ParseInbound extracts the user text messages from a verified Cloud API webhook
// body. It returns only well-formed text messages (non-empty from, id, and body);
// status-only callbacks and non-text messages yield zero messages (not an error),
// so the handler acks them fast. A body that is not valid JSON is a terminal
// parse error.
//
// The body MUST already have passed signature verification — parse only after
// VerifySignature succeeds (validate-then-parse).
func ParseInbound(body []byte) ([]InboundMessage, error) {
	var env webhookEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("whatsapp: parse webhook body: %w", err)
	}

	var out []InboundMessage
	for _, entry := range env.Entry {
		for _, change := range entry.Changes {
			for _, m := range change.Value.Messages {
				// Only user text messages drive a turn. Ignore other types
				// (image/audio/interactive/etc. handled later) and status callbacks
				// (which carry no messages[] entries at all).
				if m.Type != "text" {
					continue
				}
				if m.From == "" || m.ID == "" || m.Text.Body == "" {
					continue
				}
				out = append(out, InboundMessage{
					From: m.From,
					ID:   m.ID,
					Text: m.Text.Body,
				})
			}
		}
	}
	return out, nil
}
