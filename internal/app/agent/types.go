package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

// RunRequest 是一次 Agent 执行的入参。
//
// 这里的字段是当前 P0 必须的最小集合；为后续 P1/P1.5 预留：
//   - AgentID / RunMode：让单 Orchestrator 同时承载多 Agent 协作和确定性 Workflow；
//   - EnabledTools：允许逐请求收窄工具集合，配合权限策略；
//   - Stream：MVP 永远 true，保留字段是为了后续支持非流式 batch 调用。
type RunRequest struct {
	OrgID          string
	ProjectID      string
	UserID         string
	ConversationID string
	Input          string
	EnabledTools   []string // 空表示使用全部已注册工具
	Model          string   // 空表示使用 Router.DefaultModel
	AgentID        string   // 预留
	RunMode        string   // 预留：single / handoff / workflow
	Stream         bool
}

// SSE 输出事件，对外承诺与 domain.EventType 一致。
type sseEvent = domain.ResponseEvent

// EventEnvelope 是发到 channel 的事件。
// Err 仅在终止前的失败路径上非 nil；Final=true 表示这是 response 的最后一条事件，
// SSE handler 收到后应关闭流。
type EventEnvelope struct {
	Event *domain.ResponseEvent
	Err   error
	Final bool
}

// RunResult 是 Run 的返回。Channel 可被 SSE handler 消费；ResponseID 同时返回，
// 便于上层在 SSE 还没写出第一条事件之前就把它写到响应头。
type RunResult struct {
	ResponseID     string
	ConversationID string
	MessageID      string
	Events         <-chan EventEnvelope
}

// Orchestrator 是单 Agent 工具循环的执行器。
//
// MVP 提供单一实现 *DefaultOrchestrator；接口保留是为了让上层 handler 可以
// 在测试中替换成 fake，并为后续多实现（多 Agent、Workflow）保持 API 稳定。
type Orchestrator interface {
	Run(ctx context.Context, req RunRequest) (*RunResult, error)
}

// modelMessage 在 orchestrator 内部使用，直接对齐 model.Message 但额外携带
// 工具调用对应的 tool_call_id 以便构造下一轮模型请求。
type modelToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// streamEventOptions 控制把 ResponseEvent 写入持久化 + 推送 channel 的便捷封装。
type streamEventOptions struct {
	Type domain.EventType
	Data any
	When time.Time
}

const (
	// 这两个常量被 Run 用来限制单次响应的资源消耗。
	defaultMaxToolIterations = 5
	defaultModelTimeout      = 60 * time.Second
)
