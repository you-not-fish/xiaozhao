package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

// userRow is the GORM row for the users table.
type userRow struct {
	ID               string    `gorm:"column:id;primaryKey;size:64"`
	Email            string    `gorm:"column:email;size:255;not null"`
	PasswordHash    string    `gorm:"column:password_hash;size:255;not null"`
	Name             string    `gorm:"column:name;size:255;not null"`
	Status           string    `gorm:"column:status;size:32;not null;default:active"`
	ExternalID       string    `gorm:"column:external_id;size:255"`
	IdentityProvider string    `gorm:"column:identity_provider;size:64"`
	CreatedAt        time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (userRow) TableName() string { return "users" }

func (r userRow) toDomain() *domain.User {
	return &domain.User{
		ID:               r.ID,
		Email:            r.Email,
		PasswordHash:     r.PasswordHash,
		Name:             r.Name,
		Status:           domain.UserStatus(r.Status),
		ExternalID:       r.ExternalID,
		IdentityProvider: r.IdentityProvider,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

func newUserRow(u *domain.User) userRow {
	return userRow{
		ID:               u.ID,
		Email:            strings.ToLower(u.Email),
		PasswordHash:     u.PasswordHash,
		Name:             u.Name,
		Status:           string(u.Status),
		ExternalID:       u.ExternalID,
		IdentityProvider: u.IdentityProvider,
		CreatedAt:        u.CreatedAt,
		UpdatedAt:        u.UpdatedAt,
	}
}

// UserRepo is the GORM-backed domain.UserRepository.
type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	row := newUserRow(u)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	row.UpdatedAt = now()
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).Where("id = ? AND status <> ?", id, domain.UserStatusDeleted).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).
		Where("lower(email) = ? AND status <> ?", strings.ToLower(email), domain.UserStatusDeleted).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	row := newUserRow(u)
	row.UpdatedAt = now()
	res := r.db.WithContext(ctx).Model(&userRow{}).Where("id = ?", u.ID).Updates(map[string]any{
		"email":             row.Email,
		"password_hash":     row.PasswordHash,
		"name":              row.Name,
		"status":            row.Status,
		"external_id":       row.ExternalID,
		"identity_provider": row.IdentityProvider,
		"updated_at":        row.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
