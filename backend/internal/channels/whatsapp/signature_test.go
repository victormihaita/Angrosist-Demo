package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// sign computes the X-Hub-Signature-256 header value for body with secret.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature_Valid(t *testing.T) {
	secret := "app-secret"
	body := []byte(`{"hello":"world"}`)
	if err := VerifySignature(secret, body, sign(secret, body)); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifySignature_Tampered(t *testing.T) {
	secret := "app-secret"
	body := []byte(`{"hello":"world"}`)
	header := sign(secret, body)

	// Tamper with the body after signing — the HMAC must no longer match.
	if err := VerifySignature(secret, []byte(`{"hello":"evil"}`), header); err == nil {
		t.Fatal("tampered body accepted")
	}

	// Tamper with the header (flip a hex digit).
	bad := []byte(header)
	if bad[len(bad)-1] == '0' {
		bad[len(bad)-1] = '1'
	} else {
		bad[len(bad)-1] = '0'
	}
	if err := VerifySignature(secret, body, string(bad)); err == nil {
		t.Fatal("tampered signature accepted")
	}
}

func TestVerifySignature_MissingOrMalformed(t *testing.T) {
	secret := "app-secret"
	body := []byte(`{}`)

	cases := map[string]string{
		"empty header":      "",
		"no prefix":         "deadbeef",
		"wrong prefix":      "sha1=deadbeef",
		"non-hex value":     signaturePrefix + "zzzz",
		"wrong length hash": signaturePrefix + "ab",
	}
	for name, header := range cases {
		if err := VerifySignature(secret, body, header); err == nil {
			t.Errorf("%s: expected rejection, got nil", name)
		}
	}
}

func TestVerifySignature_NoSecretRejects(t *testing.T) {
	body := []byte(`{}`)
	if err := VerifySignature("", body, sign("x", body)); err == nil {
		t.Fatal("unconfigured secret must reject (no bypass)")
	}
}

func TestVerifyChallengeToken(t *testing.T) {
	if !VerifyChallengeToken("tok", "tok") {
		t.Fatal("matching token rejected")
	}
	if VerifyChallengeToken("tok", "nope") {
		t.Fatal("mismatched token accepted")
	}
	if VerifyChallengeToken("", "") {
		t.Fatal("empty configured token must never match")
	}
	if VerifyChallengeToken("tok", "") {
		t.Fatal("empty presented token must not match")
	}
}
