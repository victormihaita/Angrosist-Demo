// Package whatsapp is the WhatsApp Cloud API channel adapter: an outbound text
// sender (Cloud API POST) and the inbound webhook concerns (signature
// verification + payload parsing) used by the webhook handler. It speaks the Meta
// Graph HTTP API directly with net/http — no Meta SDK — and reads every
// vendor-specific value (base URL, token, phone number id, app secret, verify
// token) from config, never hardcoded.
//
// Real WhatsApp traffic is gated on Meta Business verification (an owner action).
// Until the env is configured and the number verified, the adapter is inert: the
// webhook routes still run signature checks and the sender returns a clear error
// rather than calling Meta.
//
// Deferred (documented TODOs, not built here):
//   - 24h customer-service window tracking and the out-of-window re-engagement
//     template send (FR-1.4). Today Send only delivers free-form text, which Meta
//     accepts only inside the 24h window.
//   - multi-vertical routing from wa.me intent prefixes (FR-1.5); inbound
//     WhatsApp conversations default to angrosist/buy for now.
package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// defaultAPIBase is the Meta Graph API base used when WHATSAPP_API_BASE is unset.
// It is a sane default version, not a secret; the value is still overridable via
// config so a version bump is a config change, not a code change.
const defaultAPIBase = "https://graph.facebook.com/v21.0"

// defaultTimeout bounds a single Cloud API call so a turn never blocks
// indefinitely on Meta.
const defaultTimeout = 10 * time.Second

// SenderConfig carries the Cloud API settings, all sourced from config/env
// (Secret Manager in prod). The sender is inert (returns ErrNotConfigured) until
// Token and PhoneNumberID are set.
type SenderConfig struct {
	// APIBase is the Graph API base URL (WHATSAPP_API_BASE). Empty -> defaultAPIBase.
	APIBase string
	// Token is the Cloud API access token (WHATSAPP_TOKEN), sent as a bearer.
	Token string
	// PhoneNumberID is the sending phone number id (WHATSAPP_PHONE_NUMBER_ID); it
	// forms the messages endpoint path.
	PhoneNumberID string
	// Timeout bounds a single send. Zero -> defaultTimeout.
	Timeout time.Duration
}

// Sender posts outbound WhatsApp text messages through the Cloud API. The HTTP
// client is injectable so tests can point it at an httptest server.
type Sender struct {
	apiBase       string
	token         string
	phoneNumberID string
	httpClient    *http.Client
}

// ErrNotConfigured is returned by Send when the Cloud API credentials are not
// set, so a misconfigured deployment fails loudly instead of silently dropping
// replies.
var ErrNotConfigured = fmt.Errorf("whatsapp: sender not configured (set WHATSAPP_TOKEN and WHATSAPP_PHONE_NUMBER_ID)")

// NewSender constructs a Cloud API sender from config with the default HTTP
// client. Use NewSenderWithClient to inject a client in tests.
func NewSender(cfg SenderConfig) *Sender {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return NewSenderWithClient(cfg, &http.Client{Timeout: timeout})
}

// NewSenderWithClient constructs a sender with a caller-supplied HTTP client
// (test seam). A nil client falls back to a default-timeout client.
func NewSenderWithClient(cfg SenderConfig, client *http.Client) *Sender {
	base := strings.TrimRight(cfg.APIBase, "/")
	if base == "" {
		base = defaultAPIBase
	}
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &Sender{
		apiBase:       base,
		token:         cfg.Token,
		phoneNumberID: cfg.PhoneNumberID,
		httpClient:    client,
	}
}

// Configured reports whether the sender has the credentials needed to call the
// Cloud API. Wiring uses it to decide whether to register the WhatsApp channel.
func (s *Sender) Configured() bool {
	return s.token != "" && s.phoneNumberID != ""
}

// textMessage is the Cloud API request body for a free-form text send.
type textMessage struct {
	MessagingProduct string   `json:"messaging_product"`
	To               string   `json:"to"`
	Type             string   `json:"type"`
	Text             textBody `json:"text"`
}

type textBody struct {
	Body string `json:"body"`
}

// Send delivers a plain-text message to toPhone (E.164, no '+' required by Meta)
// via POST {apiBase}/{phoneNumberID}/messages. It returns ErrNotConfigured when
// credentials are missing, and wraps transport/non-2xx failures with context. It
// logs no PII (no phone number, no body) — the caller logs ids/hashes only.
//
// NOTE (deferred): free-form text is only accepted by Meta inside the 24h
// customer-service window. Out-of-window re-engagement must use an approved
// template (FR-1.4); that path is not built yet.
func (s *Sender) Send(ctx context.Context, toPhone, text string) error {
	if !s.Configured() {
		return ErrNotConfigured
	}

	payload := textMessage{
		MessagingProduct: "whatsapp",
		To:               toPhone,
		Type:             "text",
		Text:             textBody{Body: text},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("whatsapp: marshal message: %w", err)
	}

	url := s.apiBase + "/" + s.phoneNumberID + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("whatsapp: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp: send returned status %d", resp.StatusCode)
	}
	return nil
}
