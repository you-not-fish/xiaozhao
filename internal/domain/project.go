package domain

import "time"

// ProjectVisibility currently has only "org" (every org member can access).
// Kept as an explicit field so later per-project ACLs can be added without a
// schema migration.
type ProjectVisibility string

const (
	ProjectVisibilityOrg ProjectVisibility = "org"
)

// Project is a workspace inside an organization. Knowledge bases,
// conversations and tool configurations will be scoped to a project.
type Project struct {
	ID         string
	OrgID      string
	Name       string
	Settings   map[string]any
	Visibility ProjectVisibility
	CreatedBy  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
