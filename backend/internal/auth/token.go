package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/angrosist/demo/internal/domain"
)

// DefaultTokenTTL is the staff session lifetime. The API contract suggests a
// short TTL; ~12h keeps a working day's session without a refresh endpoint
// (refresh is a later deliverable).
const DefaultTokenTTL = 12 * time.Hour

// ErrInvalidToken is returned by TokenIssuer.Verify for any token that is
// missing, malformed, expired, tampered, or signed with the wrong secret.
var ErrInvalidToken = errors.New("invalid token")

// Claims is the staff JWT payload: sub = user id, role = dashboard role, plus the
// standard registered claims (exp, iat).
type Claims struct {
	Role domain.Role `json:"role"`
	jwt.RegisteredClaims
}

// TokenIssuer issues and verifies HS256 staff JWTs. Construct it with NewTokenIssuer.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenIssuer builds a TokenIssuer from the HMAC signing secret. It returns an
// error when the secret is empty, so the server fails fast at startup rather than
// minting unsigned-equivalent tokens. There is intentionally no dev default.
func NewTokenIssuer(secret string) (*TokenIssuer, error) {
	if secret == "" {
		return nil, errors.New("auth: JWT_SECRET is required (no default permitted)")
	}
	return &TokenIssuer{
		secret: []byte(secret),
		ttl:    DefaultTokenTTL,
		now:    time.Now,
	}, nil
}

// Issue mints a signed JWT for the user with sub=user.ID and role=user.Role,
// expiring after the issuer's TTL.
func (t *TokenIssuer) Issue(user *domain.User) (string, error) {
	now := t.now()
	claims := Claims{
		Role: user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, nil
}

// Verify parses and validates a token string, enforcing the HS256 algorithm and
// expiry. On success it returns the claims; any failure yields ErrInvalidToken.
func (t *TokenIssuer) Verify(tokenStr string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(tokenStr, &claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", tok.Header["alg"])
		}
		return t.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithTimeFunc(t.now))
	if err != nil {
		return nil, ErrInvalidToken
	}
	if claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return &claims, nil
}
