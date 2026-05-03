package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

type projectRow struct {
	ID         string         `gorm:"column:id;primaryKey;size:64"`
	OrgID      string         `gorm:"column:org_id;size:64;not null"`
	Name       string         `gorm:"column:name;size:255;not null"`
	Settings   datatypes.JSON `gorm:"column:settings;type:jsonb;not null;default:'{}'::jsonb"`
	Visibility string         `gorm:"column:visibility;size:32;not null;default:org"`
	CreatedBy  string         `gorm:"column:created_by;size:64;not null"`
	CreatedAt  time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt  time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (projectRow) TableName() string { return "projects" }

func (r projectRow) toDomain() (*domain.Project, error) {
	settings, err := unmarshalSettings([]byte(r.Settings))
	if err != nil {
		return nil, err
	}
	return &domain.Project{
		ID:         r.ID,
		OrgID:      r.OrgID,
		Name:       r.Name,
		Settings:   settings,
		Visibility: domain.ProjectVisibility(r.Visibility),
		CreatedBy:  r.CreatedBy,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}, nil
}

func newProjectRow(p *domain.Project) (projectRow, error) {
	settings, err := marshalSettings(p.Settings)
	if err != nil {
		return projectRow{}, err
	}
	vis := p.Visibility
	if vis == "" {
		vis = domain.ProjectVisibilityOrg
	}
	return projectRow{
		ID:         p.ID,
		OrgID:      p.OrgID,
		Name:       p.Name,
		Settings:   datatypes.JSON(settings),
		Visibility: string(vis),
		CreatedBy:  p.CreatedBy,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	}, nil
}

// ProjectRepo is the GORM-backed domain.ProjectRepository.
type ProjectRepo struct {
	db *gorm.DB
}

func NewProjectRepo(db *gorm.DB) *ProjectRepo { return &ProjectRepo{db: db} }

func (r *ProjectRepo) Create(ctx context.Context, p *domain.Project) error {
	row, err := newProjectRow(p)
	if err != nil {
		return err
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	row.UpdatedAt = now()
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrAlreadyExists
		}
		if isFKViolation(err) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

func (r *ProjectRepo) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	var row projectRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return row.toDomain()
}

func (r *ProjectRepo) ListByOrg(ctx context.Context, orgID string) ([]domain.Project, error) {
	var rows []projectRow
	err := r.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Project, 0, len(rows))
	for _, row := range rows {
		p, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func (r *ProjectRepo) Update(ctx context.Context, p *domain.Project) error {
	row, err := newProjectRow(p)
	if err != nil {
		return err
	}
	row.UpdatedAt = now()
	res := r.db.WithContext(ctx).Model(&projectRow{}).Where("id = ?", p.ID).Updates(map[string]any{
		"name":       row.Name,
		"settings":   row.Settings,
		"visibility": row.Visibility,
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

func (r *ProjectRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&projectRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
