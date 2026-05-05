package domain

import "time"

// EventType 响应事件类型枚举。
// 保持与 OpenAI Responses 事件命名贴近，便于未来对接 SDK。
type EventType string

const (
	EventResponseCreated   EventType = "response.created"
	EventMessageDelta      EventType = "message.delta"
	EventMessageCompleted  EventType = "message.completed"
	EventToolCallCreated   EventType = "tool_call.created"
	EventToolCallDelta     EventType = "tool_call.delta"
	EventToolCallCompleted EventType = "tool_call.completed"
	EventToolCallFailed    EventType = "tool_call.failed"
	EventCitationAdded     EventType = "citation.added"
	EventUsageUpdated      EventType = "usage.updated"
	EventResponseCompleted EventType = "response.completed"
	EventResponseFailed    EventType = "response.failed"
)

// ResponseEvent 是 Agent 运行过程中写入的事件记录。
// 事件必须先持久化再推送 SSE，保证前端断线后仍可回放。
type ResponseEvent struct {
	ID             string
	ResponseID     string
	ConversationID string
	OrgID          string
	ProjectID      string
	Seq            int
	Type           EventType
	Data           []byte // JSON 编码的事件 payload
	CreatedAt      time.Time
}

// ResponseStatus 响应生命周期状态。
type ResponseStatus string

const (
	ResponseStatusPending    ResponseStatus = "pending"
	ResponseStatusInProgress ResponseStatus = "in_progress"
	ResponseStatusCompleted  ResponseStatus = "completed"
	ResponseStatusFailed     ResponseStatus = "failed"
)

// ModelInvocation 记录一次模型调用的成本与结果，独立于 response_events。
// 同一个 response 可能触发多次模型调用（工具循环每一轮一次）。
type ModelInvocation struct {
	ID            string
	OrgID         string
	ProjectID     string
	ResponseID    string
	ModelName     string
	Provider      string
	InputTokens   int
	OutputTokens  int
	LatencyMS     int
	Status        string // succeeded / failed
	ErrorCode     string
	EstimatedCost float64
	CreatedAt     time.Time
}

// ToolCallStatus 工具调用状态。
type ToolCallStatus string

const (
	ToolCallStatusPending   ToolCallStatus = "pending"
	ToolCallStatusRunning   ToolCallStatus = "running"
	ToolCallStatusSucceeded ToolCallStatus = "succeeded"
	ToolCallStatusFailed    ToolCallStatus = "failed"
)

// ToolCall 是一次工具调用的审计记录。
type ToolCall struct {
	ID           string
	OrgID        string
	ProjectID    string
	ResponseID   string
	ToolID       string
	ToolName     string
	ArgsHash     string
	ArgsRedacted map[string]any
	Status       ToolCallStatus
	LatencyMS    int
	ErrorCode    string
	CreatedAt    time.Time
}

// ToolResult 是工具调用的结果；大结果走 ResultRef 指向对象存储。
type ToolResult struct {
	ID             string
	ToolCallID     string
	ResultRef      string
	ResultSummary  string
	ResultRedacted map[string]any
	Truncated      bool
	CreatedAt      time.Time
}

// TraceSpan 是一次 Agent 运行中某个阶段的性能/调试 span。
type TraceSpan struct {
	ID                 string
	TraceID            string
	ParentSpanID       string
	OrgID              string
	ProjectID          string
	ResponseID         string
	SpanType           string // agent_run / model / tool / guardrail / rag
	Name               string
	Status             string // ok / error
	StartedAt          time.Time
	EndedAt            time.Time
	DurationMS         int
	AttributesRedacted map[string]any
}

// AuditLog 是合规追责用的审计条目。
type AuditLog struct {
	ID           string
	OrgID        string
	ProjectID    string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	IP           string
	UserAgent    string
	Metadata     map[string]any
	CreatedAt    time.Time
}

// UsageRecord 是一次资源消耗的流水记录。
type UsageRecord struct {
	ID            string
	OrgID         string
	ProjectID     string
	UserID        string
	SourceType    string // model / tool / search / embedding
	SourceID      string
	Quantity      float64
	Unit          string
	EstimatedCost float64
	CreatedAt     time.Time
}
