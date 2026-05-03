// Package password provides bcrypt-based password hashing and verification.
// The cost is fixed at bcrypt.DefaultCost; callers may bump it via env/config
// in a future iteration if hashing latency becomes a concern.
package password

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrMismatch = errors.New("password: mismatch")
)

// Hash returns the bcrypt hash of plain.
func Hash(plain string) (string, error) {
	if plain == "" {
		return "", fmt.Errorf("password: empty")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("password: hash: %w", err)
	}
	return string(h), nil
}

// Verify reports nil if plain matches hash; otherwise ErrMismatch (or another
// bcrypt error for malformed hashes).
func Verify(hash, plain string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrMismatch
		}
		return fmt.Errorf("password: verify: %w", err)
	}
	return nil
}
