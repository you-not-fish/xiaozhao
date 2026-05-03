// Package org implements organization and membership management. Creation
// automatically enrolls the creator as owner so a fresh account always has
// one tenant to work in.
package org

import (
	"context"
	"errors"
	"strings"

	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

type Service struct {
	orgs    domain.OrganizationRepository
	members domain.OrgMemberRepository
	users   domain.UserRepository
	rbac    *rbac.Checker
}

func NewService(
	orgs domain.OrganizationRepository,
	members domain.OrgMemberRepository,
	users domain.UserRepository,
	rbacChecker *rbac.Checker,
) *Service {
	return &Service{orgs: orgs, members: members, users: users, rbac: rbacChecker}
}

// Create provisions a new organization with the creator as owner.
// Membership insertion happens in a best-effort sequence: if it fails we roll
// back the org row so we never leave an ownerless org behind.
func (s *Service) Create(ctx context.Context, userID, name, plan string) (*domain.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "name is required")
	}
	if plan == "" {
		plan = "free"
	}
	o := &domain.Organization{
		ID:        id.New(id.PrefixOrg),
		Name:      name,
		Plan:      plan,
		Status:    domain.OrgStatusActive,
		CreatedBy: userID,
	}
	if err := s.orgs.Create(ctx, o); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create organization")
	}
	member := &domain.OrgMember{
		OrgID:  o.ID,
		UserID: userID,
		Role:   domain.RoleOwner,
		Status: domain.MemberStatusActive,
	}
	if err := s.members.Upsert(ctx, member); err != nil {
		// Attempt cleanup so we don't orphan the organization.
		o.Status = domain.OrgStatusDeleted
		_ = s.orgs.Update(ctx, o)
		return nil, errcode.Wrap(err, errcode.CodeInternal, "enroll owner")
	}
	return o, nil
}

// ListForUser returns every active org the user belongs to.
func (s *Service) ListForUser(ctx context.Context, userID string) ([]domain.Organization, error) {
	out, err := s.orgs.ListForUser(ctx, userID)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "list organizations")
	}
	return out, nil
}

// Get returns an org the user is a member of.
func (s *Service) Get(ctx context.Context, userID, orgID string) (*domain.Organization, error) {
	if _, err := s.rbac.Require(ctx, orgID, userID, domain.RoleViewer); err != nil {
		return nil, err
	}
	o, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeOrgNotFound, "organization not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "get organization")
	}
	return o, nil
}

// Rename updates the organization display name. Admin or owner required.
func (s *Service) Rename(ctx context.Context, userID, orgID, newName string) (*domain.Organization, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "name is required")
	}
	if _, err := s.rbac.Require(ctx, orgID, userID, domain.RoleAdmin); err != nil {
		return nil, err
	}
	o, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeOrgNotFound, "organization not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "get organization")
	}
	o.Name = newName
	if err := s.orgs.Update(ctx, o); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "rename organization")
	}
	return o, nil
}

// ListMembers returns every membership in an org. Any active member may list.
func (s *Service) ListMembers(ctx context.Context, userID, orgID string) ([]domain.OrgMember, error) {
	if _, err := s.rbac.Require(ctx, orgID, userID, domain.RoleViewer); err != nil {
		return nil, err
	}
	out, err := s.members.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "list members")
	}
	return out, nil
}

// InviteOrUpdateMember adds a user to the org or updates their role.
// The caller must be at least admin. Target role is capped at admin so
// only owners can transfer ownership (handled separately in a later iteration).
func (s *Service) InviteOrUpdateMember(ctx context.Context, actorID, orgID, targetUserID string, role domain.Role) (*domain.OrgMember, error) {
	if !role.Valid() {
		return nil, errcode.New(errcode.CodeInvalidArgument, "invalid role")
	}
	if role == domain.RoleOwner {
		return nil, errcode.New(errcode.CodeInvalidArgument, "owner transfer is not supported in MVP")
	}
	if _, err := s.rbac.Require(ctx, orgID, actorID, domain.RoleAdmin); err != nil {
		return nil, err
	}
	if _, err := s.users.GetByID(ctx, targetUserID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeNotFound, "target user not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "lookup user")
	}
	m := &domain.OrgMember{
		OrgID:  orgID,
		UserID: targetUserID,
		Role:   role,
		Status: domain.MemberStatusActive,
	}
	if err := s.members.Upsert(ctx, m); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "upsert member")
	}
	return m, nil
}

// RemoveMember detaches a user from an org. Owners cannot be removed.
func (s *Service) RemoveMember(ctx context.Context, actorID, orgID, targetUserID string) error {
	if _, err := s.rbac.Require(ctx, orgID, actorID, domain.RoleAdmin); err != nil {
		return err
	}
	target, err := s.members.Get(ctx, orgID, targetUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errcode.New(errcode.CodeNotFound, "membership not found")
		}
		return errcode.Wrap(err, errcode.CodeInternal, "get member")
	}
	if target.Role == domain.RoleOwner {
		return errcode.New(errcode.CodeForbidden, "cannot remove the organization owner")
	}
	if err := s.members.Remove(ctx, orgID, targetUserID); err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "remove member")
	}
	return nil
}

// IsMember is used by auth.SwitchOrg to validate the active-org change.
// Error mapping matches rbac.Require so handlers see consistent codes.
func (s *Service) IsMember(ctx context.Context, orgID, userID string) error {
	_, err := s.rbac.Require(ctx, orgID, userID, domain.RoleViewer)
	return err
}
