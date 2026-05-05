// Package tool 是 Tool Router 的实现。它为所有 Agent 可用的工具
// （builtin / http / mcp / sandbox）提供统一注册、调度、参数校验、
// 超时控制和审计入口。MVP 只启用 builtin 类型，但数据结构为未来
// 类型保留了扩展点。
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RiskLevel 风险等级：L0 = 纯读取/确定性无副作用，L1 = 有限读取，
// L2/L3 预留给可能引发成本/副作用的工具（MVP 不落地）。
type RiskLevel string

const (
	RiskL0 RiskLevel = "L0"
	RiskL1 RiskLevel = "L1"
	RiskL2 RiskLevel = "L2"
	RiskL3 RiskLevel = "L3"
)

// Spec 是工具自描述结构，给模型与 Orchestrator 使用。
type Spec struct {
	Name          string          // 唯一名称，模型调用时用
	Description   string          // 传给模型的自然语言描述
	Category      string          // 展示用分类
	RiskLevel     RiskLevel       // 风险等级
	InputSchema   json.RawMessage // JSON Schema；模型据此生成参数
	OutputSchema  json.RawMessage // JSON Schema；前端/审计据此理解结果
	TimeoutMS     int             // 单次执行超时
	ProviderType  string          // builtin / http / mcp / sandbox
}

// Tool 是可被 Orchestrator 执行的工具抽象。
type Tool interface {
	Spec() Spec
	// Execute 的返回值必须可 JSON 序列化；失败返回 error。
	// 实现应尽量产出"可总结"的 map 结构，避免把大文本直接返回给模型。
	Execute(ctx context.Context, args json.RawMessage) (any, error)
}

// Registry 是工具注册表；按名称索引，并发读安全（MVP 期全部在启动期注册）。
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册一个工具；重复名字会返回错误以便启动期尽早暴露配置冲突。
func (r *Registry) Register(t Tool) error {
	name := t.Spec().Name
	if name == "" {
		return errors.New("tool: empty name")
	}
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool: duplicate name %q", name)
	}
	r.tools[name] = t
	return nil
}

// Get 按名称查找；找不到返回 nil, false。
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List 返回所有已注册工具（按注册顺序并非稳定，调用方如需稳定请排序）。
func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// Specs 返回所有已注册工具的 Spec，供 Orchestrator 传给模型。
func (r *Registry) Specs() []Spec {
	out := make([]Spec, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Spec())
	}
	return out
}

// Execute 是对 Tool.Execute 的带超时包装；返回 (result, latency, error)。
// Orchestrator 应该走这里而不是直接调 Tool.Execute，这样超时策略集中一处。
func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (any, time.Duration, error) {
	start := time.Now()
	t, ok := r.Get(name)
	if !ok {
		return nil, 0, fmt.Errorf("tool %q not found", name)
	}
	spec := t.Spec()
	timeout := time.Duration(spec.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type resp struct {
		res any
		err error
	}
	ch := make(chan resp, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- resp{err: fmt.Errorf("tool panic: %v", r)}
			}
		}()
		res, err := t.Execute(execCtx, args)
		ch <- resp{res: res, err: err}
	}()

	select {
	case out := <-ch:
		return out.res, time.Since(start), out.err
	case <-execCtx.Done():
		return nil, time.Since(start), fmt.Errorf("tool %q timed out after %s: %w", name, timeout, execCtx.Err())
	}
}
