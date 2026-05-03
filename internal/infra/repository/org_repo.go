package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

type orgRow struct {
	ID        string    `gorm:"column:id;primaryKey;size:64"`
	Name      string    `gorm:"column:name;size:255;not null"`
	Plan      string    `gorm:"column:plan;size:64;not null;default:free"`
	Status    string    `gorm:"column:status;size:32;not null;default:active"`
	CreatedBy string    `gorm:"column:created_by;size:64;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (orgRow) TableName() string { return "organizations" }

func (r orgRow) toDomain() *domain.Organization {
	return &domain.Organization{
		ID:        r.ID,
		Name:      r.Name,
		Plan:      r.Plan,
		Status:    domain.OrgStatus(r.Status),
		CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func newOrgRow(o *domain.Organization) orgRow {
	return orgRow{
		ID:        o.ID,
		Name:      o.Name,
		Plan:      o.Plan,
		Status:    string(o.Status),
		CreatedBy: o.CreatedBy,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
}

// OrgRepo is the GORM-backed domain.OrganizationRepository.
type OrgRepo struct {
	db *gorm.DB
}

func NewOrgRepo(db *gorm.DB) *OrgRepo { return &OrgRepo{db: db} }

func (r *OrgRepo) Create(ctx context.Context, o *domain.Organization) error {
	row := newOrgRow(o)
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

func (r *OrgRepo) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	var row orgRow
	err := r.db.WithContext(ctx).Where("id = ? AND status <> ?", id, domain.OrgStatusDeleted).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *OrgRepo) ListForUser(ctx context.Context, userID string) ([]domain.Organization, error) {
	var rows []orgRow
	err := r.db.WithContext(ctx).
		Table("organizations AS o").
		Select("o.*").
		Joins("INNER JOIN org_members m ON m.org_id = o.id").
		Where("m.user_id = ? AND m.status = ? AND o.status <> ?",
			userID, domain.MemberStatusActive, domain.OrgStatusDeleted).
		Order("o.created_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row.toDomain())
	}
	return out, nil
}

func (r *OrgRepo) Update(ctx context.Context, o *domain.Organization) error {
	row := newOrgRow(o)
	row.UpdatedAt = now()
	res := r.db.WithContext(ctx).Model(&orgRow{}).Where("id = ?", o.ID).Updates(map[string]any{
		"name":       row.Name,
		"plan":       row.Plan,
		"status":     row.Status,
		"updated_at": row.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
