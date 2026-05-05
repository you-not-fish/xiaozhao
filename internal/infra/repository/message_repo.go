package repository

import (
	"context"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

// ========== messages & message_items ==========

type messageRow struct {
	ID             string    `gorm:"column:id;primaryKey;size:64"`
	ConversationID string    `gorm:"column:conversation_id;size:64;not null"`
	ResponseID     string    `gorm:"column:response_id;size:64"`
	Role           string    `gorm:"column:role;size:16;not null"`
	Status         string    `gorm:"column:status;size:32;not null;default:completed"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (messageRow) TableName() string { return "messages" }

type messageItemRow struct {
	ID        string         `gorm:"column:id;primaryKey;size:64"`
	MessageID string         `gorm:"column:message_id;size:64;not null"`
	Seq       int            `gorm:"column:seq;not null;default:0"`
	Type      string         `gorm:"column:type;size:32;not null"`
	Content   datatypes.JSON `gorm:"column:content;type:jsonb;not null;default:'{}'::jsonb"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (messageItemRow) TableName() string { return "message_items" }

// MessageRepo 是 GORM 仓储实现。
type MessageRepo struct{ db *gorm.DB }

func NewMessageRepo(db *gorm.DB) *MessageRepo { return &MessageRepo{db: db} }

func (r *MessageRepo) CreateMessage(ctx context.Context, m *domain.Message) error {
	row := messageRow{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		ResponseID:     m.ResponseID,
		Role:           string(m.Role),
		Status:         string(m.Status),
		CreatedAt:      m.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *MessageRepo) UpdateMessageStatus(ctx context.Context, id string, status domain.MessageStatus) error {
	res := r.db.WithContext(ctx).Model(&messageRow{}).Where("id = ?", id).
		Update("status", string(status))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// AppendItem 追加消息项；content / metadata 允许为空 map。
func (r *MessageRepo) AppendItem(ctx context.Context, item *domain.MessageItem) error {
	content, err := marshalSettings(item.Content)
	if err != nil {
		return err
	}
	meta, err := marshalSettings(item.Metadata)
	if err != nil {
		return err
	}
	row := messageItemRow{
		ID:        item.ID,
		MessageID: item.MessageID,
		Seq:       item.Seq,
		Type:      string(item.Type),
		Content:   datatypes.JSON(content),
		Metadata:  datatypes.JSON(meta),
		CreatedAt: item.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *MessageRepo) ListByConversation(ctx context.Context, convID string, limit int) ([]domain.Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []messageRow
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Message{
			ID:             row.ID,
			ConversationID: row.ConversationID,
			ResponseID:     row.ResponseID,
			Role:           domain.MessageRole(row.Role),
			Status:         domain.MessageStatus(row.Status),
			CreatedAt:      row.CreatedAt,
		})
	}
	return out, nil
}

func (r *MessageRepo) ListItemsByMessage(ctx context.Context, messageID string) ([]domain.MessageItem, error) {
	var rows []messageItemRow
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Order("seq ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.MessageItem, 0, len(rows))
	for _, row := range rows {
		content, _ := unmarshalSettings([]byte(row.Content))
		meta, _ := unmarshalSettings([]byte(row.Metadata))
		out = append(out, domain.MessageItem{
			ID:        row.ID,
			MessageID: row.MessageID,
			Seq:       row.Seq,
			Type:      domain.MessageItemType(row.Type),
			Content:   content,
			Metadata:  meta,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}
