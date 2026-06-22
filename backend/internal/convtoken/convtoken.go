// Package convtoken issues and verifies stateless conversation-ownership tokens.
//
// A visitor who knows or guesses a server-generated conversation UUID must not be
// able to continue, read, or upload to someone else's conversation. The public
// chat surface is unauthenticated, so we bind the right to act on a conversation
// to an opaque token that only the server can mint (SECURITY.md §1.1 Spoofing:
// "server-issued opaque conversation token; never trust client-supplied
// conversation_id without a server-side ownership check").
//
// The token is a keyed HMAC of the conversation id and carries no secret of its
// own beyond unforgeability:
//
//	token(convID) = base64url(HMAC-SHA256(key, "conv:" + convID))
//
// It is stateless (no DB row, no migration): any instance with the same key can
// verify a token another instance issued. The key comes from
// CONVERSATION_TOKEN_SECRET when set, otherwise it is derived from the already
// required JWT_SECRET with domain separation, so no new required secret is added.
//
// Hexagonal: this package is pure (crypto + encoding only, no infrastructure
// imports). The Issuer is constructed in the composition root and injected into
// the HTTP edge; handlers depend on it, not on a global.
package convtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// domainSeparation namespaces the JWT_SECRET-derived key so a conversation token
// can never be confused with, or substituted for, a staff session JWT signature.
const domainSeparation = "conversation-token"

// convPrefix domain-separates the HMAC message itself, binding the token to the
// conversation namespace.
const convPrefix = "conv:"

// Issuer mints and verifies conversation-ownership tokens. It is safe for
// concurrent use. Construct it with New.
type Issuer struct {
	key []byte
}

// New builds an Issuer from the conversation-token signing key.
//
// If conversationSecret is non-empty it is used directly (the operator set
// CONVERSATION_TOKEN_SECRET). Otherwise the key is derived from jwtSecret with
// domain separation — HMAC-SHA256(jwtSecret, "conversation-token") — so the
// feature works with no new required secret while remaining cryptographically
// distinct from the staff-JWT signing key.
//
// Both inputs may be empty (e.g. minimal test wiring); the resulting Issuer still
// produces deterministic, self-consistent tokens. Production wiring always has a
// JWT_SECRET (auth fails fast without it), so the key is never the empty-input
// degenerate case there.
func New(conversationSecret, jwtSecret string) *Issuer {
	var key []byte
	if conversationSecret != "" {
		key = []byte(conversationSecret)
	} else {
		mac := hmac.New(sha256.New, []byte(jwtSecret))
		mac.Write([]byte(domainSeparation))
		key = mac.Sum(nil)
	}
	return &Issuer{key: key}
}

// Issue returns the conversation-ownership token for convID. The same convID and
// key always yield the same token (it is a deterministic MAC, not a random
// session id), so any instance can re-issue and any instance can verify.
func (i *Issuer) Issue(convID string) string {
	return base64.RawURLEncoding.EncodeToString(i.mac(convID))
}

// Verify reports whether token is the valid conversation-ownership token for
// convID. It uses a constant-time comparison (hmac.Equal) so a caller cannot
// learn the correct token byte-by-byte through timing. A malformed (non-base64)
// token, a token for a different convID, or a token minted under a different key
// all return false.
func (i *Issuer) Verify(convID, token string) bool {
	got, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	return hmac.Equal(got, i.mac(convID))
}

// mac computes HMAC-SHA256(key, "conv:"+convID).
func (i *Issuer) mac(convID string) []byte {
	mac := hmac.New(sha256.New, i.key)
	mac.Write([]byte(convPrefix))
	mac.Write([]byte(convID))
	return mac.Sum(nil)
}
