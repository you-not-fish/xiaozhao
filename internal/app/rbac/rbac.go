// Package rbac centralises role-based access checks. Services never hardcode
// role comparisons; they depend on these functions so new rules (per-project
// ACL, ABAC) can land in one place.
package rbac

import (
	"context"
	"errors"

	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

// Checker resolves the membership between a user and an organization and
// gates access based on role rank.
type Checker struct {
	members domain.OrgMemberRepository
}

func NewChecker(members domain.OrgMemberRepository) *Checker {
	return &Checker{members: members}
}

// Require returns the user's membership if they hold at least the required
// role in the given organization. Errors are already translated to the API
// error code layer.
func (c *Checker) Require(ctx context.Context, orgID, userID string, min domain.Role) (*domain.OrgMember, error) {
	if orgID == "" || userID == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "org_id and user_id are required")
	}
	m, err := c.members.Get(ctx, orgID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeOrgNotMember, "not a member of this organization")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "check membership")
	}
	if m.Status != domain.MemberStatusActive {
		return nil, errcode.New(errcode.CodeOrgNotMember, "membership is not active")
	}
	if !m.Role.AtLeast(min) {
		return nil, errcode.Newf(errcode.CodeInsufficientRole,
			"role %s is below required %s", m.Role, min).
			WithMetadata("required_role", string(min)).
			WithMetadata("current_role", string(m.Role))
	}
	return m, nil
}
