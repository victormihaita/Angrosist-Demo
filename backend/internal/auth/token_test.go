package auth

import (
	"testing"
	"time"

	"github.com/angrosist/demo/internal/domain"
)

func testUser() *domain.User {
	return &domain.User{ID: "11111111-1111-1111-1111-111111111111", Role: domain.RoleStaff}
}

func TestNewTokenIssuer_RequiresSecret(t *testing.T) {
	if _, err := NewTokenIssuer(""); err == nil {
		t.Fatal("expected error for empty secret (no dev default allowed)")
	}
	if _, err := NewTokenIssuer("s3cr3t"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTokenIssuer_IssueVerifyRoundtrip(t *testing.T) {
	iss, err := NewTokenIssuer("super-secret-signing-key")
	if err != nil {
		t.Fatal(err)
	}
	u := testUser()
	tok, err := iss.Issue(u)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := iss.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != u.ID {
		t.Fatalf("sub = %q, want %q", claims.Subject, u.ID)
	}
	if claims.Role != domain.RoleStaff {
		t.Fatalf("role = %q, want staff", claims.Role)
	}
}

func TestTokenIssuer_RejectsTampered(t *testing.T) {
	iss, _ := NewTokenIssuer("super-secret-signing-key")
	tok, _ := iss.Issue(testUser())
	// Flip the last character of the signature.
	tampered := tok[:len(tok)-1] + flip(tok[len(tok)-1])
	if _, err := iss.Verify(tampered); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestTokenIssuer_RejectsWrongSecret(t *testing.T) {
	a, _ := NewTokenIssuer("secret-A")
	b, _ := NewTokenIssuer("secret-B")
	tok, _ := a.Issue(testUser())
	if _, err := b.Verify(tok); err == nil {
		t.Fatal("expected token signed with another secret to be rejected")
	}
}

func TestTokenIssuer_RejectsExpired(t *testing.T) {
	iss, _ := NewTokenIssuer("super-secret-signing-key")
	// Issue at a time well in the past so the token is already expired now.
	past := time.Now().Add(-24 * time.Hour)
	iss.now = func() time.Time { return past }
	tok, _ := iss.Issue(testUser())

	iss.now = time.Now // verification happens "now"
	if _, err := iss.Verify(tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func flip(b byte) string {
	if b == 'a' {
		return "b"
	}
	return "a"
}
