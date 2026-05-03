package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

type orgMemberRow struct {
	OrgID    string    `gorm:"column:org_id;primaryKey;size:64"`
	UserID   string    `gorm:"column:user_id;primaryKey;size:64"`
	Role     string    `gorm:"column:role;size:32;not null"`
	Status   string    `gorm:"column:status;size:32;not null;default:active"`
	JoinedAt time.Time `gorm:"column:joined_at;not null;default:now()"`
}

func (orgMemberRow) TableName() string { return "org_members" }

func (r orgMemberRow) toDomain() *domain.OrgMember {
	return &domain.OrgMember{
		OrgID:    r.OrgID,
		UserID:   r.UserID,
		Role:     domain.Role(r.Role),
		Status:   domain.MemberStatus(r.Status),
		JoinedAt: r.JoinedAt,
	}
}

func newMemberRow(m *domain.OrgMember) orgMemberRow {
	return orgMemberRow{
		OrgID:    m.OrgID,
		UserID:   m.UserID,
		Role:     string(m.Role),
		Status:   string(m.Status),
		JoinedAt: m.JoinedAt,
	}
}

// OrgMemberRepo is the GORM-backed domain.OrgMemberRepository.
type OrgMemberRepo struct {
	db *gorm.DB
}

func NewOrgMemberRepo(db *gorm.DB) *OrgMemberRepo { return &OrgMemberRepo{db: db} }

// Upsert inserts a membership or, if it exists, updates role/status.
// This matches the common "invite or re-enable member" flow without requiring
// the caller to decide between INSERT and UPDATE up-front.
func (r *OrgMemberRepo) Upsert(ctx context.Context, m *domain.OrgMember) error {
	row := newMemberRow(m)
	if row.JoinedAt.IsZero() {
		row.JoinedAt = now()
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role", "status"}),
	}).Create(&row).Error
	if err != nil {
		if isFKViolation(err) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

func (r *OrgMemberRepo) Get(ctx context.Context, orgID, userID string) (*domain.OrgMember, error) {
	var row orgMemberRow
	err := r.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *OrgMemberRepo) ListByOrg(ctx context.Context, orgID string) ([]domain.OrgMember, error) {
	var rows []orgMemberRow
	err := r.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("joined_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.OrgMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row.toDomain())
	}
	return out, nil
}

func (r *OrgMemberRepo) Remove(ctx context.Context, orgID, userID string) error {
	res := r.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Delete(&orgMemberRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
