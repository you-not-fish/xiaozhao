// Package domain contains the core entities of the tenant layer and the
// repository interfaces the app layer depends on. Entities are deliberately
// plain Go structs with no persistence-specific tags; infrastructure-only
// metadata lives in the infra/repository layer so we can swap the storage
// engine without rewriting the domain.
package domain

import "time"

// UserStatus enumerates user lifecycle states.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusDeleted  UserStatus = "deleted"
)

// User is the canonical account entity. PasswordHash is only non-empty for
// email/password logins; SSO users may carry ExternalID + IdentityProvider
// instead (reserved for future use).
type User struct {
	ID               string
	Email            string
	PasswordHash     string
	Name             string
	Status           UserStatus
	ExternalID       string
	IdentityProvider string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Sanitized returns a copy with password-related fields stripped.
// Handlers should always return the sanitized form in API responses.
func (u User) Sanitized() User {
	u.PasswordHash = ""
	return u
}
