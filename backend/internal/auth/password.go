// Package auth provides password hashing and JWT issuing/verification for the
// staff/admin dashboard. It is infrastructure-free: it depends only on the
// domain (for roles) and on standard crypto libraries, so the HTTP layer and the
// container can wire it without coupling the domain to a vendor.
package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is the single error surfaced for any login failure
// (unknown user, password-less account, wrong password). Callers map it to a
// generic 401 so they cannot reveal which check failed.
var ErrInvalidCredentials = errors.New("invalid credentials")

// HashPassword returns the bcrypt hash of the plaintext password using the
// default cost. The plaintext is never stored or logged.
func HashPassword(plaintext string) (string, error) {
	if plaintext == "" {
		return "", errors.New("auth: empty password")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword reports whether the plaintext matches the stored bcrypt hash.
// An empty hash (NULL password_hash in the DB) always fails, so accounts without
// a password cannot be logged into.
func VerifyPassword(hash, plaintext string) bool {
	if hash == "" || plaintext == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
