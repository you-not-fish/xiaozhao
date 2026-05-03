package domain

import "context"

// UserRepository persists User aggregates.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, u *User) error
}

// OrganizationRepository persists Organization aggregates.
type OrganizationRepository interface {
	Create(ctx context.Context, o *Organization) error
	GetByID(ctx context.Context, id string) (*Organization, error)
	ListForUser(ctx context.Context, userID string) ([]Organization, error)
	Update(ctx context.Context, o *Organization) error
}

// OrgMemberRepository persists OrgMember aggregates.
type OrgMemberRepository interface {
	Upsert(ctx context.Context, m *OrgMember) error
	Get(ctx context.Context, orgID, userID string) (*OrgMember, error)
	ListByOrg(ctx context.Context, orgID string) ([]OrgMember, error)
	Remove(ctx context.Context, orgID, userID string) error
}

// ProjectRepository persists Project aggregates.
type ProjectRepository interface {
	Create(ctx context.Context, p *Project) error
	GetByID(ctx context.Context, id string) (*Project, error)
	ListByOrg(ctx context.Context, orgID string) ([]Project, error)
	Update(ctx context.Context, p *Project) error
	Delete(ctx context.Context, id string) error
}
