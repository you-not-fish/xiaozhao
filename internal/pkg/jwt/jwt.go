// Package jwt issues and validates access tokens used by the API gateway.
// Tokens are HS256-signed and carry the user id plus optional active org
// context. The signing secret is read from config and never logged.
package jwt

import (
	"errors"
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// Claims is the structured payload embedded in a token.
type Claims struct {
	UserID string `json:"sub"`
	OrgID  string `json:"org_id,omitempty"` // optional active org context
	gojwt.RegisteredClaims
}

// Manager issues and verifies tokens.
type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

func NewManager(secret, issuer string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), issuer: issuer, ttl: ttl}
}

// Issue creates a signed access token for the given user.
// orgID may be empty when the user has not selected an active organization yet.
func (m *Manager) Issue(userID, orgID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.ttl)
	claims := Claims{
		UserID: userID,
		OrgID:  orgID,
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			ExpiresAt: gojwt.NewNumericDate(expiresAt),
			IssuedAt:  gojwt.NewNumericDate(now),
			NotBefore: gojwt.NewNumericDate(now.Add(-time.Second)),
		},
	}
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: sign: %w", err)
	}
	return signed, expiresAt, nil
}

// Errors returned by Parse.
var (
	ErrInvalid = errors.New("jwt: invalid token")
	ErrExpired = errors.New("jwt: token expired")
)

// Parse validates a signed token and returns its claims.
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	tok, err := gojwt.ParseWithClaims(tokenStr, claims, func(t *gojwt.Token) (any, error) {
		if _, ok := t.Method.(*gojwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, gojwt.WithIssuer(m.issuer))
	if err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !tok.Valid {
		return nil, ErrInvalid
	}
	return claims, nil
}
