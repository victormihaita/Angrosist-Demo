package convtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestIssueVerifyRoundtrip(t *testing.T) {
	iss := New("", "jwt-secret")
	const convID = "11111111-1111-1111-1111-111111111111"

	tok := iss.Issue(convID)
	if tok == "" {
		t.Fatal("Issue returned empty token")
	}
	if !iss.Verify(convID, tok) {
		t.Fatalf("Verify rejected a freshly issued token")
	}
}

func TestVerifyRejectsWrongToken(t *testing.T) {
	iss := New("", "jwt-secret")
	const convID = "11111111-1111-1111-1111-111111111111"

	if iss.Verify(convID, "not-a-valid-token") {
		t.Fatal("Verify accepted a garbage token")
	}
	if iss.Verify(convID, "") {
		t.Fatal("Verify accepted an empty token")
	}
	// A token for a tampered (off-by-one) convID must not verify against convID.
	other := iss.Issue("22222222-2222-2222-2222-222222222222")
	if iss.Verify(convID, other) {
		t.Fatal("Verify accepted a token issued for a different conversation")
	}
}

func TestVerifyRejectsWrongConvID(t *testing.T) {
	iss := New("conv-secret", "")
	tok := iss.Issue("conv-a")
	if iss.Verify("conv-b", tok) {
		t.Fatal("a token bound to conv-a verified for conv-b")
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	const convID = "conv-x"
	a := New("secret-a", "")
	b := New("secret-b", "")
	tok := a.Issue(convID)
	if b.Verify(convID, tok) {
		t.Fatal("a token minted under secret-a verified under secret-b")
	}
}

// TestExplicitSecretOverridesJWTDerivation proves CONVERSATION_TOKEN_SECRET, when
// set, takes precedence over the JWT-derived key (different tokens for the same
// convID under the two key sources).
func TestExplicitSecretOverridesJWTDerivation(t *testing.T) {
	const convID = "conv-y"
	explicit := New("explicit-secret", "jwt-secret")
	derived := New("", "jwt-secret")
	if explicit.Issue(convID) == derived.Issue(convID) {
		t.Fatal("explicit conversation secret did not override the JWT-derived key")
	}
}

// TestJWTDerivationDeterministicAndDomainSeparated proves the derived key is a
// deterministic, domain-separated function of JWT_SECRET (so any instance with the
// same JWT_SECRET verifies the same tokens) and matches the documented derivation.
func TestJWTDerivationDeterministicAndDomainSeparated(t *testing.T) {
	const (
		jwtSecret = "the-jwt-secret"
		convID    = "conv-z"
	)
	a := New("", jwtSecret)
	b := New("", jwtSecret)
	if a.Issue(convID) != b.Issue(convID) {
		t.Fatal("JWT-derived issuer is not deterministic across instances")
	}

	// Reproduce the documented derivation independently and confirm the token.
	km := hmac.New(sha256.New, []byte(jwtSecret))
	km.Write([]byte(domainSeparation))
	key := km.Sum(nil)
	mm := hmac.New(sha256.New, key)
	mm.Write([]byte(convPrefix))
	mm.Write([]byte(convID))
	want := base64.RawURLEncoding.EncodeToString(mm.Sum(nil))
	if got := a.Issue(convID); got != want {
		t.Fatalf("token = %q; want %q (documented derivation mismatch)", got, want)
	}
}
