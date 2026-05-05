package repository

import (
	"context"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

// ========== response_events ==========

type responseEventRow struct {
	ID             string         `gorm:"column:id;primaryKey;size:64"`
	ResponseID     string         `gorm:"column:response_id;size:64;not null"`
	ConversationID string         `gorm:"column:conversation_id;size:64;not null"`
	OrgID          string         `gorm:"column:org_id;size:64;not null"`
	ProjectID      string         `gorm:"column:project_id;size:64;not null"`
	Seq            int            `gorm:"column:seq;not null"`
	Type           string         `gorm:"column:type;size:64;not null"`
	Data           datatypes.JSON `gorm:"column:data;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt      time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (responseEventRow) TableName() string { return "response_events" }

// ResponseEventRepo 是 GORM 仓储实现。
// 约定：同一 response 的事件 seq 单调递增；Append 之前由 Orchestrator 侧锁住
// 当前 response 的 seq 序列分发（避免并发写冲突），数据库端以 UNIQUE(response_id, seq) 兜底。
type ResponseEventRepo struct{ db *gorm.DB }

func NewResponseEventRepo(db *gorm.DB) *ResponseEventRepo { return &ResponseEventRepo{db: db} }

func (r *ResponseEventRepo) Append(ctx context.Context, e *domain.ResponseEvent) error {
	data := e.Data
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	row := responseEventRow{
		ID:             e.ID,
		ResponseID:     e.ResponseID,
		ConversationID: e.ConversationID,
		OrgID:          e.OrgID,
		ProjectID:      e.ProjectID,
		Seq:            e.Seq,
		Type:           string(e.Type),
		Data:           datatypes.JSON(data),
		CreatedAt:      e.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *ResponseEventRepo) ListByResponse(ctx context.Context, responseID string) ([]domain.ResponseEvent, error) {
	var rows []responseEventRow
	err := r.db.WithContext(ctx).
		Where("response_id = ?", responseID).
		Order("seq ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.ResponseEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.ResponseEvent{
			ID:             row.ID,
			ResponseID:     row.ResponseID,
			ConversationID: row.ConversationID,
			OrgID:          row.OrgID,
			ProjectID:      row.ProjectID,
			Seq:            row.Seq,
			Type:           domain.EventType(row.Type),
			Data:           []byte(row.Data),
			CreatedAt:      row.CreatedAt,
		})
	}
	return out, nil
}
