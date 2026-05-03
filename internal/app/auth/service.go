// Package auth orchestrates registration and login. It is deliberately thin:
// password hashing and JWT issuance live in the pkg layer; the service just
// coordinates repositories and returns sanitized results.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
	"github.com/xiaozhao/xiaozhao/internal/pkg/jwt"
	"github.com/xiaozhao/xiaozhao/internal/pkg/password"
)

// RegisterInput is the normalized request for Service.Register.
type RegisterInput struct {
	Email    string
	Password string
	Name     string
}

// LoginInput is the normalized request for Service.Login.
type LoginInput struct {
	Email    string
	Password string
}

// TokenPair is the issued credential returned by Register/Login.
type TokenPair struct {
	AccessToken string
	ExpiresAt   time.Time
}

// Service wires repositories and the JWT manager together.
type Service struct {
	users             domain.UserRepository
	jwt               *jwt.Manager
	passwordMinLength int
}

func NewService(users domain.UserRepository, jwtMgr *jwt.Manager, passwordMinLength int) *Service {
	if passwordMinLength < 6 {
		passwordMinLength = 6
	}
	return &Service{users: users, jwt: jwtMgr, passwordMinLength: passwordMinLength}
}

// Register creates a new account and immediately issues an access token so
// the SPA does not need a follow-up login call.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.User, *TokenPair, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	name := strings.TrimSpace(in.Name)
	if email == "" || !strings.Contains(email, "@") {
		return nil, nil, errcode.New(errcode.CodeInvalidArgument, "invalid email")
	}
	if name == "" {
		name = defaultNameFromEmail(email)
	}
	if err := s.validatePassword(in.Password); err != nil {
		return nil, nil, err
	}

	if existing, err := s.users.GetByEmail(ctx, email); err == nil && existing != nil {
		return nil, nil, errcode.New(errcode.CodeAuthEmailTaken, "email already registered")
	} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, nil, errcode.Wrap(err, errcode.CodeInternal, "lookup user")
	}

	hash, err := password.Hash(in.Password)
	if err != nil {
		return nil, nil, errcode.Wrap(err, errcode.CodeInternal, "hash password")
	}
	u := &domain.User{
		ID:           id.New(id.PrefixUser),
		Email:        email,
		PasswordHash: hash,
		Name:         name,
		Status:       domain.UserStatusActive,
	}
	if err := s.users.Create(ctx, u); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return nil, nil, errcode.New(errcode.CodeAuthEmailTaken, "email already registered")
		}
		return nil, nil, errcode.Wrap(err, errcode.CodeInternal, "create user")
	}
	tok, err := s.issue(u.ID, "")
	if err != nil {
		return nil, nil, err
	}
	sanitized := u.Sanitized()
	return &sanitized, tok, nil
}

// Login verifies credentials and issues a fresh access token.
func (s *Service) Login(ctx context.Context, in LoginInput) (*domain.User, *TokenPair, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	if email == "" || in.Password == "" {
		return nil, nil, errcode.New(errcode.CodeAuthInvalidCredentials, "invalid credentials")
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Do not leak whether the email exists.
			return nil, nil, errcode.New(errcode.CodeAuthInvalidCredentials, "invalid credentials")
		}
		return nil, nil, errcode.Wrap(err, errcode.CodeInternal, "lookup user")
	}
	if u.Status != domain.UserStatusActive {
		return nil, nil, errcode.New(errcode.CodeAuthInvalidCredentials, "invalid credentials")
	}
	if err := password.Verify(u.PasswordHash, in.Password); err != nil {
		if errors.Is(err, password.ErrMismatch) {
			return nil, nil, errcode.New(errcode.CodeAuthInvalidCredentials, "invalid credentials")
		}
		return nil, nil, errcode.Wrap(err, errcode.CodeInternal, "verify password")
	}
	tok, err := s.issue(u.ID, "")
	if err != nil {
		return nil, nil, err
	}
	sanitized := u.Sanitized()
	return &sanitized, tok, nil
}

// Me returns the sanitized user behind an access token.
func (s *Service) Me(ctx context.Context, userID string) (*domain.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.New(errcode.CodeUnauthorized, "user no longer exists")
		}
		return nil, errcode.Wrap(err, errcode.CodeInternal, "lookup user")
	}
	sanitized := u.Sanitized()
	return &sanitized, nil
}

// SwitchOrg re-issues a token with the given org as the active context after
// verifying the user is still a member. We keep this on the auth service so
// the JWT continues to be the single source of truth for "active org".
func (s *Service) SwitchOrg(ctx context.Context, userID, orgID string, isMember func(ctx context.Context, orgID, userID string) error) (*TokenPair, error) {
	if orgID == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "org_id is required")
	}
	if err := isMember(ctx, orgID, userID); err != nil {
		return nil, err
	}
	return s.issue(userID, orgID)
}

func (s *Service) issue(userID, orgID string) (*TokenPair, error) {
	tok, expiresAt, err := s.jwt.Issue(userID, orgID)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "issue token")
	}
	return &TokenPair{AccessToken: tok, ExpiresAt: expiresAt}, nil
}

func (s *Service) validatePassword(p string) error {
	if len(p) < s.passwordMinLength {
		return errcode.Newf(errcode.CodeAuthPasswordWeak,
			"password must be at least %d characters", s.passwordMinLength)
	}
	var hasLetter, hasDigit bool
	for _, r := range p {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errcode.New(errcode.CodeAuthPasswordWeak,
			"password must contain letters and digits")
	}
	return nil
}

func defaultNameFromEmail(email string) string {
	if idx := strings.Index(email, "@"); idx > 0 {
		return email[:idx]
	}
	return email
}
