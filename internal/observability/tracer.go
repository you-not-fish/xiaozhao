// Package observability 提供 Agent 运行期的 trace/span 记录能力。
// MVP 先用内置仓储（trace_spans 表）承载 span 数据；后续可以改为
// 写入 OpenTelemetry pipeline，本包接口保持稳定。
//
// 使用方式：
//   tr := tracer.StartSpan(ctx, "agent_run", tracer.WithResponseID(...))
//   defer tr.End()
//   ...
//   childCtx := tracer.ContextWithParent(ctx, tr)
//   m := tracer.StartSpan(childCtx, "model.chat")
//   defer m.End()
package observability

import (
	"context"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

type ctxKey struct{}

var parentKey ctxKey

// Tracer 负责写入 span。
type Tracer struct {
	repo domain.TraceSpanRepository
}

// NewTracer 构造。
func NewTracer(repo domain.TraceSpanRepository) *Tracer { return &Tracer{repo: repo} }

// SpanOption 是可选 span 元数据。
type SpanOption func(*Span)

// WithResponseID 给 span 绑定 response_id。
func WithResponseID(rid string) SpanOption { return func(s *Span) { s.ResponseID = rid } }

// WithType 设置 span 类型（agent_run/model/tool/guardrail/rag）。
func WithType(t string) SpanOption { return func(s *Span) { s.SpanType = t } }

// WithTenant 绑定多租户标签。
func WithTenant(orgID, projectID string) SpanOption {
	return func(s *Span) { s.OrgID = orgID; s.ProjectID = projectID }
}

// Span 是一次操作的 trace 范围；End 调用时写入仓储。
type Span struct {
	tracer     *Tracer
	ID         string
	TraceID    string
	ParentID   string
	OrgID      string
	ProjectID  string
	ResponseID string
	SpanType   string
	Name       string
	StartedAt  time.Time
	Attrs      map[string]any
	Status     string
	ended      bool
}

// StartSpan 从 ctx 派生一个新 span；若 ctx 没有父 span，则新建 trace。
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) *Span {
	span := &Span{
		tracer:    t,
		ID:        id.New(id.PrefixSpan),
		Name:      name,
		SpanType:  "generic",
		StartedAt: time.Now(),
		Attrs:     map[string]any{},
		Status:    "ok",
	}
	if parent, ok := ctx.Value(parentKey).(*Span); ok && parent != nil {
		span.TraceID = parent.TraceID
		span.ParentID = parent.ID
		span.OrgID = parent.OrgID
		span.ProjectID = parent.ProjectID
		span.ResponseID = parent.ResponseID
	} else {
		span.TraceID = id.New(id.PrefixTrace)
	}
	for _, opt := range opts {
		opt(span)
	}
	return span
}

// ContextWithSpan 把 span 作为父 span 注入 ctx，用于启动子 span。
func ContextWithSpan(ctx context.Context, s *Span) context.Context {
	return context.WithValue(ctx, parentKey, s)
}

// SetAttr 追加属性；属性在 End 时一次性写入仓储。
func (s *Span) SetAttr(k string, v any) *Span {
	if s == nil {
		return s
	}
	s.Attrs[k] = v
	return s
}

// MarkError 标记 span 为错误，并记录 error message 作为属性。
func (s *Span) MarkError(err error) *Span {
	if s == nil || err == nil {
		return s
	}
	s.Status = "error"
	s.Attrs["error"] = err.Error()
	return s
}

// End 结束 span 并写入仓储。二次调用安全：只会落一次。
// 写入失败不返回错误——trace 不应该阻断主流程。
func (s *Span) End() {
	if s == nil || s.ended {
		return
	}
	s.ended = true
	now := time.Now()
	rec := &domain.TraceSpan{
		ID:                 s.ID,
		TraceID:            s.TraceID,
		ParentSpanID:       s.ParentID,
		OrgID:              s.OrgID,
		ProjectID:          s.ProjectID,
		ResponseID:         s.ResponseID,
		SpanType:           s.SpanType,
		Name:               s.Name,
		Status:             s.Status,
		StartedAt:          s.StartedAt,
		EndedAt:            now,
		DurationMS:         int(now.Sub(s.StartedAt) / time.Millisecond),
		AttributesRedacted: s.Attrs,
	}
	// 脱离请求上下文写入（防止上游 ctx cancel 导致写入失败）。
	_ = s.tracer.repo.Create(context.Background(), rec)
}

// TraceID 返回 span 所属 trace id；常用于把 trace_id 透传到日志和审计。
func (s *Span) TraceIDValue() string {
	if s == nil {
		return ""
	}
	return s.TraceID
}
