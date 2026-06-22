package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// signaturePrefix is the algorithm tag Meta prefixes to the X-Hub-Signature-256
// header value (e.g. "sha256=ab12...").
const signaturePrefix = "sha256="

// ErrSignatureInvalid is returned when the inbound webhook signature is missing,
// malformed, or does not match the HMAC of the raw body. The handler maps it to
// a 403 and logs a security event (no body content).
var ErrSignatureInvalid = errors.New("whatsapp: invalid webhook signature")

// VerifySignature checks the X-Hub-Signature-256 header against an HMAC-SHA256 of
// the raw request body keyed by the app secret, using a constant-time compare.
// The body MUST be the exact raw bytes Meta sent (compute before JSON parsing).
//
// It returns ErrSignatureInvalid for a missing/malformed header or a mismatch,
// and nil only on an exact match.
func VerifySignature(appSecret string, body []byte, header string) error {
	if appSecret == "" {
		// No secret configured: we cannot verify, so we must reject (no bypass).
		return ErrSignatureInvalid
	}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, signaturePrefix) {
		return ErrSignatureInvalid
	}
	gotHex := strings.TrimPrefix(header, signaturePrefix)
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return ErrSignatureInvalid
	}

	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	want := mac.Sum(nil)

	// hmac.Equal is constant-time and length-safe.
	if !hmac.Equal(got, want) {
		return ErrSignatureInvalid
	}
	return nil
}

// VerifyChallengeToken compares a presented hub.verify_token to the configured
// verify token using a constant-time compare. It returns true only on an exact
// match (and false when either side is empty, so an unconfigured token can never
// be satisfied).
func VerifyChallengeToken(configured, presented string) bool {
	if configured == "" || presented == "" {
		return false
	}
	return hmac.Equal([]byte(configured), []byte(presented))
}
