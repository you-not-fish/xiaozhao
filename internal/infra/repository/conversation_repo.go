package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

// ========== conversations ==========

type conversationRow struct {
	ID        string         `gorm:"column:id;primaryKey;size:64"`
	OrgID     string         `gorm:"column:org_id;size:64;not null"`
	ProjectID string         `gorm:"column:project_id;size:64;not null"`
	UserID    string         `gorm:"column:user_id;size:64;not null"`
	Title     string         `gorm:"column:title;size:255;not null;default:''"`
	Status    string         `gorm:"column:status;size:32;not null;default:active"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (conversationRow) TableName() string { return "conversations" }

func (r conversationRow) toDomain() (*domain.Conversation, error) {
	md, err := unmarshalSettings([]byte(r.Metadata))
	if err != nil {
		return nil, err
	}
	return &domain.Conversation{
		ID:        r.ID,
		OrgID:     r.OrgID,
		ProjectID: r.ProjectID,
		UserID:    r.UserID,
		Title:     r.Title,
		Status:    domain.ConversationStatus(r.Status),
		Metadata:  md,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}, nil
}

func newConversationRow(c *domain.Conversation) (conversationRow, error) {
	md, err := marshalSettings(c.Metadata)
	if err != nil {
		return conversationRow{}, err
	}
	return conversationRow{
		ID:        c.ID,
		OrgID:     c.OrgID,
		ProjectID: c.ProjectID,
		UserID:    c.UserID,
		Title:     c.Title,
		Status:    string(c.Status),
		Metadata:  datatypes.JSON(md),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}, nil
}

// ConversationRepo 是 GORM 仓储实现。
type ConversationRepo struct{ db *gorm.DB }

func NewConversationRepo(db *gorm.DB) *ConversationRepo { return &ConversationRepo{db: db} }

func (r *ConversationRepo) Create(ctx context.Context, c *domain.Conversation) error {
	row, err := newConversationRow(c)
	if err != nil {
		return err
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	row.UpdatedAt = now()
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *ConversationRepo) GetByID(ctx context.Context, id string) (*domain.Conversation, error) {
	var row conversationRow
	err := r.db.WithContext(ctx).
		Where("id = ? AND status <> ?", id, domain.ConversationStatusDeleted).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return row.toDomain()
}

func (r *ConversationRepo) ListByProject(ctx context.Context, projectID, userID string, limit int) ([]domain.Conversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []conversationRow
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND user_id = ? AND status <> ?", projectID, userID, domain.ConversationStatusDeleted).
		Order("updated_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Conversation, 0, len(rows))
	for _, row := range rows {
		c, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

func (r *ConversationRepo) Update(ctx context.Context, c *domain.Conversation) error {
	row, err := newConversationRow(c)
	if err != nil {
		return err
	}
	row.UpdatedAt = now()
	res := r.db.WithContext(ctx).Model(&conversationRow{}).Where("id = ?", c.ID).Updates(map[string]any{
		"title":      row.Title,
		"status":     row.Status,
		"metadata":   row.Metadata,
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

func (r *ConversationRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Model(&conversationRow{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": domain.ConversationStatusDeleted, "updated_at": now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
