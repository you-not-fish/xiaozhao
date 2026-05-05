// Package model 是 Model Gateway 层，为上层 Agent Orchestrator 提供
// 统一的模型调用抽象。任何供应商（OpenAI 国际、国内 OpenAI-compatible、
// 未来 Claude / Gemini）都要实现 Provider 接口，Orchestrator 不感知具体实现。
//
// 设计要点：
//   1. 请求/响应统一：ChatRequest / Chunk，错误码通过 errcode 做标准化映射；
//   2. 只暴露流式接口：非流式场景由调用方折叠 Chunk 成最终结果，
//      这样可以避免为同一个供应商维护两套代码；
//   3. Provider 不负责工具执行——只负责把模型"想调用哪个工具、参数什么"
//      解析出来交还 Orchestrator。工具的实际执行走 Tool Router。
package model

import (
	"context"
	"encoding/json"
	"time"
)

// Role 是模型侧消息角色，与 domain.MessageRole 语义对齐。
// 在 model 层单独定义是为了让 model 层无需依赖 domain 包。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 是传给模型的一条上下文消息。
// ToolCalls 仅 assistant 消息会用到；ToolCallID 仅 tool 消息会用到。
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // assistant 产生的工具调用
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool 消息对应的调用 id
}

// ToolCall 是模型返回的一个工具调用请求。
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"` // 模型给的参数 JSON
}

// ToolSpec 描述可供模型调用的工具；对应 OpenAI function calling 的 function 字段。
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ChatRequest 是统一的模型调用请求。
type ChatRequest struct {
	Model     string
	Messages  []Message
	Tools     []ToolSpec
	MaxTokens int
	Metadata  map[string]string // trace_id / org_id / project_id / user_id 等标签
}

// FinishReason 完成原因；影响 Orchestrator 的下一步决策。
type FinishReason string

const (
	FinishStop      FinishReason = "stop"       // 模型正常结束
	FinishToolCalls FinishReason = "tool_calls" // 模型请求调用工具
	FinishLength    FinishReason = "length"     // 触达 max_tokens
	FinishError     FinishReason = "error"      // 供应商错误
)

// Usage 是单次调用的 token 用量。
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Chunk 是一次流式响应中的一个增量块。
// 同一个 Chunk 可同时携带 TextDelta 与 ToolCallDelta；字段全空则为心跳。
type Chunk struct {
	TextDelta     string
	ToolCallDelta []ToolCallDelta
	Usage         *Usage
	FinishReason  FinishReason
}

// ToolCallDelta 是工具调用的分片。
// 供应商通常按 Index 给工具调用编号，再分片下发 Name / ArgumentsDelta。
type ToolCallDelta struct {
	Index          int
	ID             string
	Name           string
	ArgumentsDelta string
}

// Stream 是 Chat 的流式返回；Recv 返回 io.EOF 表示流正常结束。
type Stream interface {
	Recv() (*Chunk, error)
	Close() error
}

// Provider 是模型供应商抽象。
// Name 返回供应商标识（用于审计与路由规则）；Chat 发起一次流式调用。
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (Stream, error)
}

// DefaultTimeout 是 Orchestrator 下发给供应商的兜底超时。
// 具体供应商可通过自身 client 配置进一步缩短。
const DefaultTimeout = 60 * time.Second
