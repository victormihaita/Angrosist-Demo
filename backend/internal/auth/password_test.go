package auth

import "testing"

func TestHashPassword_Roundtrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == "correct horse battery staple" {
		t.Fatal("hash must not equal plaintext")
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("VerifyPassword should accept the correct password")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("VerifyPassword should reject the wrong password")
	}
}

func TestHashPassword_Empty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error hashing empty password")
	}
}

func TestVerifyPassword_EmptyHash(t *testing.T) {
	// A NULL/absent password_hash (empty string) must never authenticate.
	if VerifyPassword("", "anything") {
		t.Fatal("empty hash must not verify")
	}
	if VerifyPassword("$2a$10$abcdefghijklmnopqrstuv", "") {
		t.Fatal("empty plaintext must not verify")
	}
}
