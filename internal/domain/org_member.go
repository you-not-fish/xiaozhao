package domain

import "time"

// Role is the RBAC role a user holds in an organization.
// owner > admin > member > viewer. Role comparisons should go through the
// AtLeast / Rank helpers below, never via raw string comparison.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

// Rank returns a monotonic integer where higher = more privileged.
// Unknown roles get -1 so they never satisfy AtLeast.
func (r Role) Rank() int {
	switch r {
	case RoleOwner:
		return 40
	case RoleAdmin:
		return 30
	case RoleMember:
		return 20
	case RoleViewer:
		return 10
	default:
		return -1
	}
}

// AtLeast reports whether the role is at least as privileged as min.
func (r Role) AtLeast(min Role) bool {
	return r.Rank() >= min.Rank()
}

// Valid reports whether the string is a known role.
func (r Role) Valid() bool { return r.Rank() > 0 }

// MemberStatus enumerates membership lifecycle states.
type MemberStatus string

const (
	MemberStatusActive   MemberStatus = "active"
	MemberStatusDisabled MemberStatus = "disabled"
)

// OrgMember binds a user to an organization with a role.
type OrgMember struct {
	OrgID    string
	UserID   string
	Role     Role
	Status   MemberStatus
	JoinedAt time.Time
}
