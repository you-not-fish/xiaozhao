package domain

import "time"

// ConversationStatus 会话状态枚举。
type ConversationStatus string

const (
	ConversationStatusActive   ConversationStatus = "active"
	ConversationStatusArchived ConversationStatus = "archived"
	ConversationStatusDeleted  ConversationStatus = "deleted"
)

// Conversation 表示一次多轮对话的容器。所有消息、响应事件都挂在它下面。
type Conversation struct {
	ID         string
	OrgID      string
	ProjectID  string
	UserID     string
	Title      string
	Status     ConversationStatus
	Metadata   map[string]any
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// MessageRole 消息角色：user / assistant / system / tool。
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

// MessageStatus 消息处理状态。
type MessageStatus string

const (
	MessageStatusInProgress MessageStatus = "in_progress"
	MessageStatusCompleted  MessageStatus = "completed"
	MessageStatusFailed     MessageStatus = "failed"
)

// Message 是会话中的单条消息。assistant 消息会关联到一次 response。
type Message struct {
	ID             string
	ConversationID string
	ResponseID     string
	Role           MessageRole
	Status         MessageStatus
	CreatedAt      time.Time
}

// MessageItemType 消息项类型；一条 assistant 消息可能由多个 item 组成。
type MessageItemType string

const (
	ItemTypeText       MessageItemType = "text"
	ItemTypeToolCall   MessageItemType = "tool_call"
	ItemTypeToolResult MessageItemType = "tool_result"
	ItemTypeCitation   MessageItemType = "citation"
	ItemTypeError      MessageItemType = "error"
)

// MessageItem 是消息的基本渲染单元，前端按 seq 组合展示。
type MessageItem struct {
	ID        string
	MessageID string
	Seq       int
	Type      MessageItemType
	Content   map[string]any
	Metadata  map[string]any
	CreatedAt time.Time
}
