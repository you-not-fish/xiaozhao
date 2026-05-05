package domain

import "context"

// ConversationRepository 持久化 Conversation 聚合。
type ConversationRepository interface {
	Create(ctx context.Context, c *Conversation) error
	GetByID(ctx context.Context, id string) (*Conversation, error)
	ListByProject(ctx context.Context, projectID, userID string, limit int) ([]Conversation, error)
	Update(ctx context.Context, c *Conversation) error
	Delete(ctx context.Context, id string) error
}

// MessageRepository 持久化 Message 与 MessageItem。
type MessageRepository interface {
	CreateMessage(ctx context.Context, m *Message) error
	UpdateMessageStatus(ctx context.Context, id string, status MessageStatus) error
	AppendItem(ctx context.Context, item *MessageItem) error
	ListByConversation(ctx context.Context, convID string, limit int) ([]Message, error)
	ListItemsByMessage(ctx context.Context, messageID string) ([]MessageItem, error)
}

// ResponseEventRepository 持久化响应事件并提供按序回放。
type ResponseEventRepository interface {
	// Append 写入下一条事件；实现需保证同一 response 内 seq 单调递增。
	Append(ctx context.Context, e *ResponseEvent) error
	ListByResponse(ctx context.Context, responseID string) ([]ResponseEvent, error)
}

// ModelInvocationRepository 记录模型调用审计。
type ModelInvocationRepository interface {
	Create(ctx context.Context, m *ModelInvocation) error
	ListByOrg(ctx context.Context, orgID string, limit int) ([]ModelInvocation, error)
	// ListByOrgProject 提供按 project 进一步过滤的视图，供管理后台使用。
	ListByOrgProject(ctx context.Context, orgID, projectID string, limit int) ([]ModelInvocation, error)
}

// ToolCallRepository 记录工具调用及其结果。
type ToolCallRepository interface {
	Create(ctx context.Context, c *ToolCall) error
	Update(ctx context.Context, c *ToolCall) error
	SaveResult(ctx context.Context, r *ToolResult) error
	ListByResponse(ctx context.Context, responseID string) ([]ToolCall, error)
	// ListByOrg 给管理后台 /v1/admin/tool_calls 使用，按 created_at 倒序分页。
	// projectID 为空时不限定项目。
	ListByOrg(ctx context.Context, orgID, projectID string, limit int) ([]ToolCall, error)
}

// TraceSpanRepository 记录 trace span。
type TraceSpanRepository interface {
	Create(ctx context.Context, s *TraceSpan) error
	ListByResponse(ctx context.Context, responseID string) ([]TraceSpan, error)
}

// AuditLogRepository 记录审计日志。
type AuditLogRepository interface {
	Create(ctx context.Context, a *AuditLog) error
	ListByOrg(ctx context.Context, orgID string, limit int) ([]AuditLog, error)
}

// UsageRepository 记录用量流水。
type UsageRepository interface {
	Create(ctx context.Context, u *UsageRecord) error
	ListByOrg(ctx context.Context, orgID string, limit int) ([]UsageRecord, error)
}
