package repository

import (
	"context"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/xiaozhao/xiaozhao/internal/domain"
)

// ========== model_invocations / tool_calls / tool_results / trace_spans / audit_logs / usage_records ==========

type modelInvocationRow struct {
	ID            string    `gorm:"column:id;primaryKey;size:64"`
	OrgID         string    `gorm:"column:org_id;size:64;not null"`
	ProjectID     string    `gorm:"column:project_id;size:64;not null"`
	ResponseID    string    `gorm:"column:response_id;size:64;not null"`
	ModelName     string    `gorm:"column:model_name;size:128;not null"`
	Provider      string    `gorm:"column:provider;size:64;not null"`
	InputTokens   int       `gorm:"column:input_tokens;not null;default:0"`
	OutputTokens  int       `gorm:"column:output_tokens;not null;default:0"`
	LatencyMS     int       `gorm:"column:latency_ms;not null;default:0"`
	Status        string    `gorm:"column:status;size:32;not null"`
	ErrorCode     string    `gorm:"column:error_code;size:64"`
	EstimatedCost float64   `gorm:"column:estimated_cost;type:numeric(18,8);not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (modelInvocationRow) TableName() string { return "model_invocations" }

// ModelInvocationRepo 记录模型调用审计。
type ModelInvocationRepo struct{ db *gorm.DB }

func NewModelInvocationRepo(db *gorm.DB) *ModelInvocationRepo { return &ModelInvocationRepo{db: db} }

func (r *ModelInvocationRepo) Create(ctx context.Context, m *domain.ModelInvocation) error {
	row := modelInvocationRow{
		ID:            m.ID,
		OrgID:         m.OrgID,
		ProjectID:     m.ProjectID,
		ResponseID:    m.ResponseID,
		ModelName:     m.ModelName,
		Provider:      m.Provider,
		InputTokens:   m.InputTokens,
		OutputTokens:  m.OutputTokens,
		LatencyMS:     m.LatencyMS,
		Status:        m.Status,
		ErrorCode:     m.ErrorCode,
		EstimatedCost: m.EstimatedCost,
		CreatedAt:     m.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *ModelInvocationRepo) ListByOrg(ctx context.Context, orgID string, limit int) ([]domain.ModelInvocation, error) {
	return r.listByOrg(ctx, orgID, "", limit)
}

// ListByOrgProject 在 org 过滤基础上再按 project 过滤；projectID 为空等价于 ListByOrg。
// 给 /v1/admin/model_invocations 管理端分页使用。
func (r *ModelInvocationRepo) ListByOrgProject(ctx context.Context, orgID, projectID string, limit int) ([]domain.ModelInvocation, error) {
	return r.listByOrg(ctx, orgID, projectID, limit)
}

func (r *ModelInvocationRepo) listByOrg(ctx context.Context, orgID, projectID string, limit int) ([]domain.ModelInvocation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := r.db.WithContext(ctx).Where("org_id = ?", orgID)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	var rows []modelInvocationRow
	err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.ModelInvocation, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.ModelInvocation{
			ID:            row.ID,
			OrgID:         row.OrgID,
			ProjectID:     row.ProjectID,
			ResponseID:    row.ResponseID,
			ModelName:     row.ModelName,
			Provider:      row.Provider,
			InputTokens:   row.InputTokens,
			OutputTokens:  row.OutputTokens,
			LatencyMS:     row.LatencyMS,
			Status:        row.Status,
			ErrorCode:     row.ErrorCode,
			EstimatedCost: row.EstimatedCost,
			CreatedAt:     row.CreatedAt,
		})
	}
	return out, nil
}

// ========== tool_calls ==========

type toolCallRow struct {
	ID           string         `gorm:"column:id;primaryKey;size:64"`
	OrgID        string         `gorm:"column:org_id;size:64;not null"`
	ProjectID    string         `gorm:"column:project_id;size:64;not null"`
	ResponseID   string         `gorm:"column:response_id;size:64;not null"`
	ToolID       string         `gorm:"column:tool_id;size:64"`
	ToolName     string         `gorm:"column:tool_name;size:128;not null"`
	ArgsHash     string         `gorm:"column:args_hash;size:64;not null;default:''"`
	ArgsRedacted datatypes.JSON `gorm:"column:args_redacted;type:jsonb;not null;default:'{}'::jsonb"`
	Status       string         `gorm:"column:status;size:32;not null"`
	LatencyMS    int            `gorm:"column:latency_ms;not null;default:0"`
	ErrorCode    string         `gorm:"column:error_code;size:64"`
	CreatedAt    time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (toolCallRow) TableName() string { return "tool_calls" }

type toolResultRow struct {
	ID             string         `gorm:"column:id;primaryKey;size:64"`
	ToolCallID     string         `gorm:"column:tool_call_id;size:64;not null"`
	ResultRef      string         `gorm:"column:result_ref;size:255"`
	ResultSummary  string         `gorm:"column:result_summary;type:text;not null;default:''"`
	ResultRedacted datatypes.JSON `gorm:"column:result_redacted;type:jsonb;not null;default:'{}'::jsonb"`
	Truncated      bool           `gorm:"column:truncated;not null;default:false"`
	CreatedAt      time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (toolResultRow) TableName() string { return "tool_results" }

// ToolCallRepo 记录工具调用与结果。
type ToolCallRepo struct{ db *gorm.DB }

func NewToolCallRepo(db *gorm.DB) *ToolCallRepo { return &ToolCallRepo{db: db} }

func (r *ToolCallRepo) Create(ctx context.Context, c *domain.ToolCall) error {
	args, err := marshalSettings(c.ArgsRedacted)
	if err != nil {
		return err
	}
	row := toolCallRow{
		ID:           c.ID,
		OrgID:        c.OrgID,
		ProjectID:    c.ProjectID,
		ResponseID:   c.ResponseID,
		ToolID:       c.ToolID,
		ToolName:     c.ToolName,
		ArgsHash:     c.ArgsHash,
		ArgsRedacted: datatypes.JSON(args),
		Status:       string(c.Status),
		LatencyMS:    c.LatencyMS,
		ErrorCode:    c.ErrorCode,
		CreatedAt:    c.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *ToolCallRepo) Update(ctx context.Context, c *domain.ToolCall) error {
	args, err := marshalSettings(c.ArgsRedacted)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&toolCallRow{}).Where("id = ?", c.ID).Updates(map[string]any{
		"status":        string(c.Status),
		"latency_ms":    c.LatencyMS,
		"error_code":    c.ErrorCode,
		"args_redacted": datatypes.JSON(args),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ToolCallRepo) SaveResult(ctx context.Context, tr *domain.ToolResult) error {
	content, err := marshalSettings(tr.ResultRedacted)
	if err != nil {
		return err
	}
	row := toolResultRow{
		ID:             tr.ID,
		ToolCallID:     tr.ToolCallID,
		ResultRef:      tr.ResultRef,
		ResultSummary:  tr.ResultSummary,
		ResultRedacted: datatypes.JSON(content),
		Truncated:      tr.Truncated,
		CreatedAt:      tr.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *ToolCallRepo) ListByResponse(ctx context.Context, responseID string) ([]domain.ToolCall, error) {
	var rows []toolCallRow
	err := r.db.WithContext(ctx).Where("response_id = ?", responseID).
		Order("created_at ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return toolCallsFromRows(rows), nil
}

// ListByOrg 供管理端 /v1/admin/tool_calls 倒序分页；projectID 可空。
func (r *ToolCallRepo) ListByOrg(ctx context.Context, orgID, projectID string, limit int) ([]domain.ToolCall, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := r.db.WithContext(ctx).Where("org_id = ?", orgID)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	var rows []toolCallRow
	if err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return toolCallsFromRows(rows), nil
}

func toolCallsFromRows(rows []toolCallRow) []domain.ToolCall {
	out := make([]domain.ToolCall, 0, len(rows))
	for _, row := range rows {
		args, _ := unmarshalSettings([]byte(row.ArgsRedacted))
		out = append(out, domain.ToolCall{
			ID:           row.ID,
			OrgID:        row.OrgID,
			ProjectID:    row.ProjectID,
			ResponseID:   row.ResponseID,
			ToolID:       row.ToolID,
			ToolName:     row.ToolName,
			ArgsHash:     row.ArgsHash,
			ArgsRedacted: args,
			Status:       domain.ToolCallStatus(row.Status),
			LatencyMS:    row.LatencyMS,
			ErrorCode:    row.ErrorCode,
			CreatedAt:    row.CreatedAt,
		})
	}
	return out
}

// ========== trace_spans ==========

type traceSpanRow struct {
	ID                 string         `gorm:"column:id;primaryKey;size:64"`
	TraceID            string         `gorm:"column:trace_id;size:64;not null"`
	ParentSpanID       string         `gorm:"column:parent_span_id;size:64"`
	OrgID              string         `gorm:"column:org_id;size:64;not null"`
	ProjectID          string         `gorm:"column:project_id;size:64;not null"`
	ResponseID         string         `gorm:"column:response_id;size:64"`
	SpanType           string         `gorm:"column:span_type;size:32;not null"`
	Name               string         `gorm:"column:name;size:128;not null"`
	Status             string         `gorm:"column:status;size:32;not null;default:ok"`
	StartedAt          time.Time      `gorm:"column:started_at;not null"`
	EndedAt            time.Time      `gorm:"column:ended_at"`
	DurationMS         int            `gorm:"column:duration_ms;not null;default:0"`
	AttributesRedacted datatypes.JSON `gorm:"column:attributes_redacted;type:jsonb;not null;default:'{}'::jsonb"`
}

func (traceSpanRow) TableName() string { return "trace_spans" }

// TraceSpanRepo 记录 trace span。
type TraceSpanRepo struct{ db *gorm.DB }

func NewTraceSpanRepo(db *gorm.DB) *TraceSpanRepo { return &TraceSpanRepo{db: db} }

func (r *TraceSpanRepo) Create(ctx context.Context, s *domain.TraceSpan) error {
	attrs, err := marshalSettings(s.AttributesRedacted)
	if err != nil {
		return err
	}
	row := traceSpanRow{
		ID:                 s.ID,
		TraceID:            s.TraceID,
		ParentSpanID:       s.ParentSpanID,
		OrgID:              s.OrgID,
		ProjectID:          s.ProjectID,
		ResponseID:         s.ResponseID,
		SpanType:           s.SpanType,
		Name:               s.Name,
		Status:             s.Status,
		StartedAt:          s.StartedAt,
		EndedAt:            s.EndedAt,
		DurationMS:         s.DurationMS,
		AttributesRedacted: datatypes.JSON(attrs),
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *TraceSpanRepo) ListByResponse(ctx context.Context, responseID string) ([]domain.TraceSpan, error) {
	var rows []traceSpanRow
	err := r.db.WithContext(ctx).Where("response_id = ?", responseID).
		Order("started_at ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.TraceSpan, 0, len(rows))
	for _, row := range rows {
		attrs, _ := unmarshalSettings([]byte(row.AttributesRedacted))
		out = append(out, domain.TraceSpan{
			ID:                 row.ID,
			TraceID:            row.TraceID,
			ParentSpanID:       row.ParentSpanID,
			OrgID:              row.OrgID,
			ProjectID:          row.ProjectID,
			ResponseID:         row.ResponseID,
			SpanType:           row.SpanType,
			Name:               row.Name,
			Status:             row.Status,
			StartedAt:          row.StartedAt,
			EndedAt:            row.EndedAt,
			DurationMS:         row.DurationMS,
			AttributesRedacted: attrs,
		})
	}
	return out, nil
}

// ========== audit_logs ==========

type auditLogRow struct {
	ID           string         `gorm:"column:id;primaryKey;size:64"`
	OrgID        string         `gorm:"column:org_id;size:64"`
	ProjectID    string         `gorm:"column:project_id;size:64"`
	ActorUserID  string         `gorm:"column:actor_user_id;size:64"`
	Action       string         `gorm:"column:action;size:128;not null"`
	ResourceType string         `gorm:"column:resource_type;size:64;not null"`
	ResourceID   string         `gorm:"column:resource_id;size:64"`
	IP           string         `gorm:"column:ip;size:64"`
	UserAgent    string         `gorm:"column:user_agent;size:255"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt    time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (auditLogRow) TableName() string { return "audit_logs" }

// AuditLogRepo 记录审计日志。
type AuditLogRepo struct{ db *gorm.DB }

func NewAuditLogRepo(db *gorm.DB) *AuditLogRepo { return &AuditLogRepo{db: db} }

func (r *AuditLogRepo) Create(ctx context.Context, a *domain.AuditLog) error {
	meta, err := marshalSettings(a.Metadata)
	if err != nil {
		return err
	}
	row := auditLogRow{
		ID:           a.ID,
		OrgID:        a.OrgID,
		ProjectID:    a.ProjectID,
		ActorUserID:  a.ActorUserID,
		Action:       a.Action,
		ResourceType: a.ResourceType,
		ResourceID:   a.ResourceID,
		IP:           a.IP,
		UserAgent:    a.UserAgent,
		Metadata:     datatypes.JSON(meta),
		CreatedAt:    a.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *AuditLogRepo) ListByOrg(ctx context.Context, orgID string, limit int) ([]domain.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []auditLogRow
	err := r.db.WithContext(ctx).Where("org_id = ?", orgID).
		Order("created_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.AuditLog, 0, len(rows))
	for _, row := range rows {
		meta, _ := unmarshalSettings([]byte(row.Metadata))
		out = append(out, domain.AuditLog{
			ID:           row.ID,
			OrgID:        row.OrgID,
			ProjectID:    row.ProjectID,
			ActorUserID:  row.ActorUserID,
			Action:       row.Action,
			ResourceType: row.ResourceType,
			ResourceID:   row.ResourceID,
			IP:           row.IP,
			UserAgent:    row.UserAgent,
			Metadata:     meta,
			CreatedAt:    row.CreatedAt,
		})
	}
	return out, nil
}

// ========== usage_records ==========

type usageRow struct {
	ID            string    `gorm:"column:id;primaryKey;size:64"`
	OrgID         string    `gorm:"column:org_id;size:64;not null"`
	ProjectID     string    `gorm:"column:project_id;size:64;not null"`
	UserID        string    `gorm:"column:user_id;size:64"`
	SourceType    string    `gorm:"column:source_type;size:32;not null"`
	SourceID      string    `gorm:"column:source_id;size:64"`
	Quantity      float64   `gorm:"column:quantity;type:numeric(18,4);not null;default:0"`
	Unit          string    `gorm:"column:unit;size:32;not null;default:token"`
	EstimatedCost float64   `gorm:"column:estimated_cost;type:numeric(18,8);not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (usageRow) TableName() string { return "usage_records" }

// UsageRepo 记录用量流水。
type UsageRepo struct{ db *gorm.DB }

func NewUsageRepo(db *gorm.DB) *UsageRepo { return &UsageRepo{db: db} }

func (r *UsageRepo) Create(ctx context.Context, u *domain.UsageRecord) error {
	row := usageRow{
		ID:            u.ID,
		OrgID:         u.OrgID,
		ProjectID:     u.ProjectID,
		UserID:        u.UserID,
		SourceType:    u.SourceType,
		SourceID:      u.SourceID,
		Quantity:      u.Quantity,
		Unit:          u.Unit,
		EstimatedCost: u.EstimatedCost,
		CreatedAt:     u.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *UsageRepo) ListByOrg(ctx context.Context, orgID string, limit int) ([]domain.UsageRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []usageRow
	err := r.db.WithContext(ctx).Where("org_id = ?", orgID).
		Order("created_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.UsageRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UsageRecord{
			ID:            row.ID,
			OrgID:         row.OrgID,
			ProjectID:     row.ProjectID,
			UserID:        row.UserID,
			SourceType:    row.SourceType,
			SourceID:      row.SourceID,
			Quantity:      row.Quantity,
			Unit:          row.Unit,
			EstimatedCost: row.EstimatedCost,
			CreatedAt:     row.CreatedAt,
		})
	}
	return out, nil
}
