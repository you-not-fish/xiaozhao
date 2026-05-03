package domain

import "time"

// OrgStatus enumerates organization lifecycle states.
type OrgStatus string

const (
	OrgStatusActive   OrgStatus = "active"
	OrgStatusDisabled OrgStatus = "disabled"
	OrgStatusDeleted  OrgStatus = "deleted"
)

// Organization represents a tenant. Every domain object except User is
// scoped to an Organization.
type Organization struct {
	ID        string
	Name      string
	Plan      string
	Status    OrgStatus
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}
