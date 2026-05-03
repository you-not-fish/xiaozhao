package domain

import "errors"

// Sentinel errors used by repositories. Application services are expected to
// translate them into the appropriate *errcode.Error before returning to the
// API layer.
var (
	ErrNotFound      = errors.New("domain: not found")
	ErrAlreadyExists = errors.New("domain: already exists")
)
