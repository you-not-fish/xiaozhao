package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
)

// Router 是"多模型网关"的 MVP 实现：根据配置选择一个 Provider 对外暴露。
// 后续扩展方向：
//   - 支持多 Provider 并按能力 / 成本做路由；
//   - 主 Provider 失败时做能力对齐的降级（model_fallback_not_compatible）；
//   - 组织级主备模型配置。
//
// MVP 不做路由，单 Provider 直出，但对外暴露的接口已与后续扩展兼容。
type Router struct {
	primary Provider
	model   string // 默认 model 名
}

// NewRouter 根据配置构造 Router。
//   - provider=mock  → 返回一个 MockProvider，用于演示和开发，不需要外部依赖。
//     为了让 Orchestrator 的"工具循环"演示看起来更真实，默认脚本安排成：
//     第一轮请求 current_time 工具；第二轮用工具结果拼出最终回答。
//   - provider=openai → 返回一个 OpenAICompatible Provider。
func NewRouter(cfg config.ModelConfig) (*Router, error) {
	switch cfg.Provider {
	case "", "mock":
		return &Router{primary: defaultMockProvider(), model: fallbackModel(cfg.DefaultModel)}, nil
	case "openai":
		p := NewOpenAICompatible(OpenAICompatibleConfig{
			Name:    cfg.OpenAI.Name,
			BaseURL: cfg.OpenAI.BaseURL,
			APIKey:  cfg.OpenAI.APIKey,
			Timeout: time.Duration(cfg.OpenAI.TimeoutSeconds) * time.Second,
		})
		return &Router{primary: p, model: fallbackModel(cfg.DefaultModel)}, nil
	default:
		return nil, fmt.Errorf("model: unsupported provider %q", cfg.Provider)
	}
}

// Primary 返回当前主 Provider。
func (r *Router) Primary() Provider { return r.primary }

// DefaultModel 返回对 Provider 调用时要使用的默认模型名。
func (r *Router) DefaultModel() string { return r.model }

// defaultMockProvider 构造一个演示用的 MockProvider。
// 脚本安排：
//  1. 模型"想看看现在几点"→ 请求 current_time 工具；
//  2. 拿到工具结果后生成最终回答（echo 回落），保证 SSE 的 message.delta
//     / tool_call.* / response.completed 全链路都能被前端观测到。
func defaultMockProvider() *MockProvider {
	return NewMockProvider(
		MockScript{
			Text: "让我先看一下当前时间，",
			ToolCalls: []ToolCall{{
				ID:        "call_mock_1",
				Name:      "current_time",
				Arguments: json.RawMessage(`{}`),
			}},
		},
		MockScript{
			Text:   "好的，已经结合当前时间给出答复。",
			Finish: FinishStop,
			Usage:  &Usage{InputTokens: 64, OutputTokens: 32},
		},
	)
}

func fallbackModel(m string) string {
	if m == "" {
		return "mock-default"
	}
	return m
}
