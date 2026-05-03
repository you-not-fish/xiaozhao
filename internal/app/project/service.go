// Package project implements project CRUD scoped to an organization.
// Project operations always run through the RBAC checker so the same role
// matrix applies consistently.
package project

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
	projects domain.ProjectRepository
	orgs     domain.OrganizationRepository
	rbac     *rbac.Checker
}

func NewService(projects domain.ProjectRepository, orgs domain.OrganizationRepository, rbacChecker *rbac.Checker) *Service {
	return &Service{projects: projects, orgs: orgs, rbac: rbacChecker}
}

// CreateInput captures the fields accepted by Create.
type CreateInput struct {
	OrgID    string
	Name     string
	Settings map[string]any
}

// Create provisions a new project inside an organization.
func (s *Service) Create(ctx context.Context, userID string, in CreateInput) (*domain.Project, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "name is required")
	}
	if _, err := s.rbac.Require(ctx, in.OrgID, userID, domain.RoleAdmin); err != nil {
		return nil, err
	}
	if _, err := s.orgs.GetByID(ctx, in.OrgID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeOrgNotFound, "organization not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "get organization")
	}
	p := &domain.Project{
		ID:         id.New(id.PrefixProject),
		OrgID:      in.OrgID,
		Name:       name,
		Settings:   in.Settings,
		Visibility: domain.ProjectVisibilityOrg,
		CreatedBy:  userID,
	}
	if err := s.projects.Create(ctx, p); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return nil, errcode.New(errcode.CodeConflict, "project name already exists in this organization")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create project")
	}
	return p, nil
}

// List returns every project in an org the user can see.
func (s *Service) List(ctx context.Context, userID, orgID string) ([]domain.Project, error) {
	if _, err := s.rbac.Require(ctx, orgID, userID, domain.RoleViewer); err != nil {
		return nil, err
	}
	out, err := s.projects.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "list projects")
	}
	return out, nil
}

// Get returns a single project the user can access.
// The project must belong to an org the user is a member of.
func (s *Service) Get(ctx context.Context, userID, projectID string) (*domain.Project, error) {
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeProjectNotFound, "project not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "get project")
	}
	if _, err := s.rbac.Require(ctx, p.OrgID, userID, domain.RoleViewer); err != nil {
		// Hide existence of project behind cross-org access.
		return nil, errcode.New(errcode.CodeProjectNotFound, "project not found")
	}
	return p, nil
}

// UpdateInput captures fields that can be patched via Update.
type UpdateInput struct {
	Name     *string
	Settings *map[string]any
}

// Update mutates name and/or settings. Admin required.
func (s *Service) Update(ctx context.Context, userID, projectID string, in UpdateInput) (*domain.Project, error) {
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeProjectNotFound, "project not found")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "get project")
	}
	if _, err := s.rbac.Require(ctx, p.OrgID, userID, domain.RoleAdmin); err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, errcode.New(errcode.CodeInvalidArgument, "name must not be empty")
		}
		p.Name = name
	}
	if in.Settings != nil {
		p.Settings = *in.Settings
	}
	if err := s.projects.Update(ctx, p); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "update project")
	}
	return p, nil
}

// Delete removes a project. Admin required.
func (s *Service) Delete(ctx context.Context, userID, projectID string) error {
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errcode.New(errcode.CodeProjectNotFound, "project not found")
		}
		return errcode.Wrap(err, errcode.CodeInternal, "get project")
	}
	if _, err := s.rbac.Require(ctx, p.OrgID, userID, domain.RoleAdmin); err != nil {
		return err
	}
	if err := s.projects.Delete(ctx, projectID); err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "delete project")
	}
	return nil
}
