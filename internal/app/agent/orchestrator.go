package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xiaozhao/xiaozhao/internal/app/tool"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/infra/model"
	"github.com/xiaozhao/xiaozhao/internal/observability"
	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

// DefaultOrchestrator 是 P0 的单 Agent 工具循环实现。
type DefaultOrchestrator struct {
	conversations *ConversationService
	messages      domain.MessageRepository
	events        domain.ResponseEventRepository
	models        model.Provider
	modelName     string
	tools         *tool.Registry
	modelCalls    domain.ModelInvocationRepository
	toolCalls     domain.ToolCallRepository
	audit         domain.AuditLogRepository
	usage         domain.UsageRepository
	tracer        *observability.Tracer
	cfg           config.AgentConfig
}

type OrchestratorDeps struct {
	Conversations *ConversationService
	Messages      domain.MessageRepository
	Events        domain.ResponseEventRepository
	ModelProvider model.Provider
	ModelName     string
	Tools         *tool.Registry
	ModelCalls    domain.ModelInvocationRepository
	ToolCalls     domain.ToolCallRepository
	Audit         domain.AuditLogRepository
	Usage         domain.UsageRepository
	Tracer        *observability.Tracer
	Config        config.AgentConfig
}

func NewDefaultOrchestrator(d OrchestratorDeps) *DefaultOrchestrator {
	cfg := d.Config
	if cfg.MaxToolIterations <= 0 {
		cfg.MaxToolIterations = defaultMaxToolIterations
	}
	if cfg.ModelTimeoutSec <= 0 {
		cfg.ModelTimeoutSec = int(defaultModelTimeout / time.Second)
	}
	if cfg.ToolTimeoutSec <= 0 {
		cfg.ToolTimeoutSec = 15
	}
	if cfg.MaxToolResultKB <= 0 {
		cfg.MaxToolResultKB = 64
	}
	return &DefaultOrchestrator{
		conversations: d.Conversations,
		messages:      d.Messages,
		events:        d.Events,
		models:        d.ModelProvider,
		modelName:     d.ModelName,
		tools:         d.Tools,
		modelCalls:    d.ModelCalls,
		toolCalls:     d.ToolCalls,
		audit:         d.Audit,
		usage:         d.Usage,
		tracer:        d.Tracer,
		cfg:           cfg,
	}
}

func (o *DefaultOrchestrator) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if strings.TrimSpace(req.Input) == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "input is required")
	}
	if req.OrgID == "" || req.ProjectID == "" || req.UserID == "" {
		return nil, errcode.New(errcode.CodeInvalidArgument, "org_id / project_id / user_id are required")
	}
	if o.models == nil {
		return nil, errcode.New(errcode.CodeModelProviderUnavailable, "model provider is unavailable")
	}
	if o.tools == nil {
		o.tools = tool.NewRegistry()
	}

	conv, err := o.conversations.EnsureConversation(ctx, EnsureInput{
		OrgID:          req.OrgID,
		UserID:         req.UserID,
		ProjectID:      req.ProjectID,
		ConversationID: req.ConversationID,
		Title:          req.Input,
	})
	if err != nil {
		return nil, err
	}

	responseID := id.New(id.PrefixResponse)
	userMsg := &domain.Message{
		ID:             id.New(id.PrefixMessage),
		ConversationID: conv.ID,
		Role:           domain.RoleUser,
		Status:         domain.MessageStatusCompleted,
		CreatedAt:      time.Now(),
	}
	if err := o.messages.CreateMessage(ctx, userMsg); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create user message")
	}
	if err := o.messages.AppendItem(ctx, &domain.MessageItem{
		ID:        id.New(id.PrefixMessage),
		MessageID: userMsg.ID,
		Seq:       1,
		Type:      domain.ItemTypeText,
		Content:   map[string]any{"text": req.Input},
		CreatedAt: time.Now(),
	}); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "append user message item")
	}

	assistantMsg := &domain.Message{
		ID:             id.New(id.PrefixMessage),
		ConversationID: conv.ID,
		ResponseID:     responseID,
		Role:           domain.RoleAssistant,
		Status:         domain.MessageStatusInProgress,
		CreatedAt:      time.Now(),
	}
	if err := o.messages.CreateMessage(ctx, assistantMsg); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "create assistant message")
	}

	out := make(chan EventEnvelope, 32)
	result := &RunResult{
		ResponseID:     responseID,
		ConversationID: conv.ID,
		MessageID:      assistantMsg.ID,
		Events:         out,
	}

	go o.run(ctx, req, conv, assistantMsg, responseID, out)
	return result, nil
}

func (o *DefaultOrchestrator) run(ctx context.Context, req RunRequest, conv *domain.Conversation, assistantMsg *domain.Message, responseID string, out chan<- EventEnvelope) {
	defer close(out)
	seq := 0
	itemSeq := 0
	modelName := req.Model
	if modelName == "" {
		modelName = o.modelName
	}
	if modelName == "" {
		modelName = "mock-default"
	}

	root := o.startSpan(ctx, "agent.run", "agent_run", responseID, req.OrgID, req.ProjectID)
	defer root.End()
	ctx = observability.ContextWithSpan(ctx, root)

	emit := func(t domain.EventType, data any) error {
		seq++
		return o.emitEvent(ctx, out, responseID, conv.ID, req.OrgID, req.ProjectID, seq, t, data)
	}

	fail := func(err error) {
		root.MarkError(err)
		_ = o.messages.UpdateMessageStatus(context.Background(), assistantMsg.ID, domain.MessageStatusFailed)
		o.recordAudit(req, responseID, "response.failed", "response", responseID, map[string]any{
			"error_code": string(errcode.CodeOf(err)),
		})
		_ = emit(domain.EventResponseFailed, map[string]any{
			"error": map[string]any{
				"code":    string(errcode.CodeOf(err)),
				"message": userFacingError(err),
			},
		})
		out <- EventEnvelope{Err: err, Final: true}
	}

	if err := emit(domain.EventResponseCreated, map[string]any{
		"response_id":     responseID,
		"conversation_id": conv.ID,
		"message_id":      assistantMsg.ID,
	}); err != nil {
		fail(err)
		return
	}
	o.recordAudit(req, responseID, "response.created", "response", responseID, map[string]any{
		"conversation_id": conv.ID,
	})

	messages, err := o.buildModelMessages(ctx, conv.ID)
	if err != nil {
		fail(err)
		return
	}
	toolSpecs := o.modelToolSpecs(req.EnabledTools)
	totalUsage := model.Usage{}
	finalText := strings.Builder{}

	for iter := 0; iter <= o.cfg.MaxToolIterations; iter++ {
		if iter == o.cfg.MaxToolIterations {
			fail(errcode.New(errcode.CodeInvalidArgument, "tool iteration limit exceeded"))
			return
		}
		text, calls, usage, err := o.callModel(ctx, req, responseID, assistantMsg.ID, modelName, messages, toolSpecs, emit)
		totalUsage.InputTokens += usage.InputTokens
		totalUsage.OutputTokens += usage.OutputTokens
		if err != nil {
			fail(err)
			return
		}
		if text != "" {
			finalText.WriteString(text)
		}
		if len(calls) == 0 {
			break
		}

		assistantToolMsg := model.Message{
			Role:      model.RoleAssistant,
			Content:   text,
			ToolCalls: calls,
		}
		messages = append(messages, assistantToolMsg)
		for _, call := range calls {
			toolResult, err := o.executeTool(ctx, req, responseID, assistantMsg.ID, call, &itemSeq, emit)
			if err != nil {
				fail(err)
				return
			}
			messages = append(messages, model.Message{
				Role:       model.RoleTool,
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    toolResult,
			})
		}
	}

	if finalText.Len() > 0 {
		itemSeq++
		if err := o.messages.AppendItem(context.Background(), &domain.MessageItem{
			ID:        id.New(id.PrefixMessage),
			MessageID: assistantMsg.ID,
			Seq:       itemSeq,
			Type:      domain.ItemTypeText,
			Content:   map[string]any{"text": finalText.String()},
			CreatedAt: time.Now(),
		}); err != nil {
			fail(errcode.Wrap(err, errcode.CodeInternal, "append assistant text item"))
			return
		}
	}
	if err := emit(domain.EventMessageCompleted, map[string]any{
		"message_id": assistantMsg.ID,
	}); err != nil {
		fail(err)
		return
	}
	if err := emit(domain.EventUsageUpdated, map[string]any{
		"input_tokens":  totalUsage.InputTokens,
		"output_tokens": totalUsage.OutputTokens,
	}); err != nil {
		fail(err)
		return
	}
	o.recordUsage(req, responseID, "model", responseID, float64(totalUsage.InputTokens+totalUsage.OutputTokens), "token")
	if err := o.messages.UpdateMessageStatus(context.Background(), assistantMsg.ID, domain.MessageStatusCompleted); err != nil {
		fail(errcode.Wrap(err, errcode.CodeInternal, "complete assistant message"))
		return
	}
	if err := emit(domain.EventResponseCompleted, map[string]any{
		"response_id": responseID,
		"status":      string(domain.ResponseStatusCompleted),
	}); err != nil {
		fail(err)
		return
	}
	o.recordAudit(req, responseID, "response.completed", "response", responseID, map[string]any{
		"conversation_id": conv.ID,
	})
	out <- EventEnvelope{Final: true}
}

func (o *DefaultOrchestrator) callModel(
	ctx context.Context,
	req RunRequest,
	responseID string,
	messageID string,
	modelName string,
	messages []model.Message,
	tools []model.ToolSpec,
	emit func(domain.EventType, any) error,
) (string, []model.ToolCall, model.Usage, error) {
	span := o.startSpan(ctx, "model.chat", "model", responseID, req.OrgID, req.ProjectID)
	defer span.End()

	callID := id.New(id.PrefixEvent)
	start := time.Now()
	modelCtx, cancel := context.WithTimeout(ctx, time.Duration(o.cfg.ModelTimeoutSec)*time.Second)
	defer cancel()
	stream, err := o.models.Chat(modelCtx, model.ChatRequest{
		Model:    modelName,
		Messages: messages,
		Tools:    tools,
		Metadata: map[string]string{
			"org_id":      req.OrgID,
			"project_id":  req.ProjectID,
			"user_id":     req.UserID,
			"response_id": responseID,
		},
	})
	if err != nil {
		span.MarkError(err)
		o.recordModelInvocation(req, responseID, modelName, 0, 0, int(time.Since(start)/time.Millisecond), "failed", errcode.CodeOf(err))
		return "", nil, model.Usage{}, normalizeModelErr(err)
	}
	defer stream.Close()

	var (
		text       strings.Builder
		usage      model.Usage
		finish     model.FinishReason
		deltaCalls = map[int]*modelToolCall{}
	)
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			span.MarkError(err)
			o.recordModelInvocation(req, responseID, modelName, usage.InputTokens, usage.OutputTokens, int(time.Since(start)/time.Millisecond), "failed", errcode.CodeOf(err))
			return text.String(), nil, usage, normalizeModelErr(err)
		}
		if chunk.TextDelta != "" {
			text.WriteString(chunk.TextDelta)
			if err := emit(domain.EventMessageDelta, map[string]any{
				"message_id": messageID,
				"delta":      chunk.TextDelta,
			}); err != nil {
				return text.String(), nil, usage, err
			}
		}
		for _, d := range chunk.ToolCallDelta {
			tc := deltaCalls[d.Index]
			if tc == nil {
				tc = &modelToolCall{}
				deltaCalls[d.Index] = tc
			}
			if d.ID != "" {
				tc.ID = d.ID
			}
			if d.Name != "" {
				tc.Name = d.Name
			}
			tc.Arguments = append(tc.Arguments, []byte(d.ArgumentsDelta)...)
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		if chunk.FinishReason != "" {
			finish = chunk.FinishReason
		}
	}

	calls := make([]model.ToolCall, 0, len(deltaCalls))
	if finish == model.FinishToolCalls || len(deltaCalls) > 0 {
		for i := 0; i < len(deltaCalls); i++ {
			tc := deltaCalls[i]
			if tc == nil {
				continue
			}
			if tc.ID == "" {
				tc.ID = fmt.Sprintf("call_%s_%d", callID, i)
			}
			if tc.Name == "" {
				return text.String(), nil, usage, errcode.New(errcode.CodeModelToolCallInvalid, "model returned tool call without name")
			}
			if len(tc.Arguments) == 0 {
				tc.Arguments = json.RawMessage(`{}`)
			}
			calls = append(calls, model.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
	}
	o.recordModelInvocation(req, responseID, modelName, usage.InputTokens, usage.OutputTokens, int(time.Since(start)/time.Millisecond), "succeeded", "")
	return text.String(), calls, usage, nil
}

func (o *DefaultOrchestrator) executeTool(
	ctx context.Context,
	req RunRequest,
	responseID string,
	messageID string,
	call model.ToolCall,
	itemSeq *int,
	emit func(domain.EventType, any) error,
) (string, error) {
	span := o.startSpan(ctx, "tool."+call.Name, "tool", responseID, req.OrgID, req.ProjectID)
	defer span.End()

	args := map[string]any{}
	_ = json.Unmarshal(call.Arguments, &args)
	execArgs := call.Arguments
	if call.Name == "knowledge_search" {
		// knowledge_search 是租户敏感工具，模型只给 query；组织/项目/用户和
		// 本次请求允许的知识库范围必须由 Orchestrator 注入，避免模型越权扩域。
		args["org_id"] = req.OrgID
		args["project_id"] = req.ProjectID
		args["user_id"] = req.UserID
		if allowed := req.ToolKnowledgeBaseIDs[call.Name]; len(allowed) > 0 {
			args["allowed_knowledge_base_ids"] = allowed
		}
		if b, err := json.Marshal(args); err == nil {
			execArgs = b
		}
	}
	argsHash := hashBytes(call.Arguments)
	tc := &domain.ToolCall{
		ID:           id.New(id.PrefixToolCall),
		OrgID:        req.OrgID,
		ProjectID:    req.ProjectID,
		ResponseID:   responseID,
		ToolName:     call.Name,
		ArgsHash:     argsHash,
		ArgsRedacted: args,
		Status:       domain.ToolCallStatusRunning,
		CreatedAt:    time.Now(),
	}
	if err := o.toolCalls.Create(context.Background(), tc); err != nil {
		return "", errcode.Wrap(err, errcode.CodeInternal, "create tool call")
	}
	// 工具调用先审计、再发事件、再执行；这样即使执行阶段崩溃，也能回放到调用意图。
	if err := emit(domain.EventToolCallCreated, map[string]any{
		"tool_call_id": call.ID,
		"audit_id":     tc.ID,
		"tool_name":    call.Name,
		"args":         args,
	}); err != nil {
		return "", err
	}
	*itemSeq = *itemSeq + 1
	_ = o.messages.AppendItem(context.Background(), &domain.MessageItem{
		ID:        id.New(id.PrefixMessage),
		MessageID: messageID,
		Seq:       *itemSeq,
		Type:      domain.ItemTypeToolCall,
		Content: map[string]any{
			"tool_call_id": call.ID,
			"tool_name":    call.Name,
			"args":         args,
		},
		CreatedAt: time.Now(),
	})

	result, latency, err := o.tools.Execute(ctx, call.Name, execArgs)
	tc.LatencyMS = int(latency / time.Millisecond)
	if err != nil {
		span.MarkError(err)
		tc.Status = domain.ToolCallStatusFailed
		tc.ErrorCode = string(toolErrCode(err))
		_ = o.toolCalls.Update(context.Background(), tc)
		_ = emit(domain.EventToolCallFailed, map[string]any{
			"tool_call_id": call.ID,
			"audit_id":     tc.ID,
			"tool_name":    call.Name,
			"error": map[string]any{
				"code":    tc.ErrorCode,
				"message": err.Error(),
			},
			"latency_ms": tc.LatencyMS,
		})
		return "", errcode.Wrap(err, toolErrCode(err), "tool execution failed")
	}

	payload, truncated, originalSize, err := o.marshalToolResult(result)
	if err != nil {
		return "", err
	}
	tc.Status = domain.ToolCallStatusSucceeded
	if err := o.toolCalls.Update(context.Background(), tc); err != nil {
		return "", errcode.Wrap(err, errcode.CodeInternal, "update tool call")
	}
	tr := &domain.ToolResult{
		ID:             id.New(id.PrefixEvent),
		ToolCallID:     tc.ID,
		ResultSummary:  summarizeResult(payload),
		ResultRedacted: map[string]any{"content": payload, "original_size_bytes": originalSize},
		Truncated:      truncated,
		CreatedAt:      time.Now(),
	}
	if err := o.toolCalls.SaveResult(context.Background(), tr); err != nil {
		return "", errcode.Wrap(err, errcode.CodeInternal, "save tool result")
	}
	citations := extractCitations(result)
	for _, citation := range citations {
		*itemSeq = *itemSeq + 1
		if err := o.messages.AppendItem(context.Background(), &domain.MessageItem{
			ID:        id.New(id.PrefixMessage),
			MessageID: messageID,
			Seq:       *itemSeq,
			Type:      domain.ItemTypeCitation,
			Content:   citation,
			CreatedAt: time.Now(),
		}); err != nil {
			return "", errcode.Wrap(err, errcode.CodeInternal, "append citation item")
		}
		// 引用作为独立事件发出，前端可以先显示工具过程，再逐条挂载可点击来源。
		if err := emit(domain.EventCitationAdded, map[string]any{
			"tool_call_id": call.ID,
			"tool_name":    call.Name,
			"citation":     citation,
		}); err != nil {
			return "", err
		}
	}
	*itemSeq = *itemSeq + 1
	_ = o.messages.AppendItem(context.Background(), &domain.MessageItem{
		ID:        id.New(id.PrefixMessage),
		MessageID: messageID,
		Seq:       *itemSeq,
		Type:      domain.ItemTypeToolResult,
		Content: map[string]any{
			"tool_call_id":        call.ID,
			"tool_name":           call.Name,
			"result":              payload,
			"truncated":           truncated,
			"original_size_bytes": originalSize,
		},
		CreatedAt: time.Now(),
	})
	if err := emit(domain.EventToolCallCompleted, map[string]any{
		"tool_call_id":        call.ID,
		"audit_id":            tc.ID,
		"tool_name":           call.Name,
		"result":              payload,
		"truncated":           truncated,
		"original_size_bytes": originalSize,
		"latency_ms":          tc.LatencyMS,
	}); err != nil {
		return "", err
	}
	o.recordUsage(req, responseID, "tool", tc.ID, 1, "call")
	return payload, nil
}

func extractCitations(result any) []map[string]any {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	out := make([]map[string]any, 0, 4)
	if c, ok := obj["citation"].(map[string]any); ok {
		out = append(out, c)
	}
	if items, ok := obj["citations"].([]any); ok {
		for _, item := range items {
			c, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, c)
			if len(out) >= 10 {
				break
			}
		}
	}
	return dedupeCitations(out)
}

func dedupeCitations(items []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	seen := map[string]bool{}
	for _, c := range items {
		key := fmt.Sprintf("%v|%v", c["source_type"], c["url"])
		if key == "|" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func (o *DefaultOrchestrator) buildModelMessages(ctx context.Context, conversationID string) ([]model.Message, error) {
	msgs, err := o.messages.ListByConversation(ctx, conversationID, 50)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeInternal, "load conversation messages")
	}
	out := []model.Message{{
		Role:    model.RoleSystem,
		Content: "你是团队知识助理。请用简洁、准确的中文回答；如果使用工具结果，请基于工具事实回答。",
	}}
	for _, m := range msgs {
		if m.Role == domain.RoleTool || m.Role == domain.RoleSystem {
			continue
		}
		items, err := o.messages.ListItemsByMessage(ctx, m.ID)
		if err != nil {
			return nil, errcode.Wrap(err, errcode.CodeInternal, "load message items")
		}
		text := messageText(items)
		if text == "" {
			continue
		}
		role := model.RoleUser
		if m.Role == domain.RoleAssistant {
			role = model.RoleAssistant
		}
		out = append(out, model.Message{Role: role, Content: text})
	}
	return out, nil
}

func (o *DefaultOrchestrator) modelToolSpecs(enabled []string) []model.ToolSpec {
	allow := map[string]bool{}
	for _, name := range enabled {
		allow[name] = true
	}
	specs := o.tools.Specs()
	out := make([]model.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if len(allow) > 0 && !allow[s.Name] {
			continue
		}
		out = append(out, model.ToolSpec{
			Name:        s.Name,
			Description: s.Description,
			Parameters:  s.InputSchema,
		})
	}
	return out
}

func (o *DefaultOrchestrator) emitEvent(ctx context.Context, out chan<- EventEnvelope, responseID, conversationID, orgID, projectID string, seq int, t domain.EventType, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "marshal response event")
	}
	event := &domain.ResponseEvent{
		ID:             id.New(id.PrefixEvent),
		ResponseID:     responseID,
		ConversationID: conversationID,
		OrgID:          orgID,
		ProjectID:      projectID,
		Seq:            seq,
		Type:           t,
		Data:           payload,
		CreatedAt:      time.Now(),
	}
	// 事件必须先持久化再推给 SSE；客户端断线后只能依赖数据库回放完整过程。
	if err := o.events.Append(context.Background(), event); err != nil {
		return errcode.Wrap(err, errcode.CodeInternal, "append response event")
	}
	select {
	case out <- EventEnvelope{Event: event}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (o *DefaultOrchestrator) marshalToolResult(result any) (string, bool, int, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return "", false, 0, errcode.Wrap(err, errcode.CodeInternal, "marshal tool result")
	}
	original := len(raw)
	limit := o.cfg.MaxToolResultKB * 1024
	if limit <= 0 || len(raw) <= limit {
		return string(raw), false, original, nil
	}
	// P0 只截断不摘要：摘要会产生额外模型调用和成本，留到 P1.5。
	// 按字节截断可能切在 UTF-8 多字节序列中间，回退到最近的有效 UTF-8 边界。
	end := limit
	for end > 0 && !utf8.Valid(raw[:end]) {
		end--
	}
	return string(raw[:end]), true, original, nil
}

func (o *DefaultOrchestrator) recordModelInvocation(req RunRequest, responseID, modelName string, inTok, outTok, latency int, status string, code errcode.Code) {
	if o.modelCalls == nil {
		return
	}
	_ = o.modelCalls.Create(context.Background(), &domain.ModelInvocation{
		ID:           id.New(id.PrefixEvent),
		OrgID:        req.OrgID,
		ProjectID:    req.ProjectID,
		ResponseID:   responseID,
		ModelName:    modelName,
		Provider:     o.models.Name(),
		InputTokens:  inTok,
		OutputTokens: outTok,
		LatencyMS:    latency,
		Status:       status,
		ErrorCode:    string(code),
		CreatedAt:    time.Now(),
	})
}

func (o *DefaultOrchestrator) recordUsage(req RunRequest, responseID, sourceType, sourceID string, quantity float64, unit string) {
	if o.usage == nil || quantity <= 0 {
		return
	}
	_ = o.usage.Create(context.Background(), &domain.UsageRecord{
		ID:         id.New(id.PrefixEvent),
		OrgID:      req.OrgID,
		ProjectID:  req.ProjectID,
		UserID:     req.UserID,
		SourceType: sourceType,
		SourceID:   sourceID,
		Quantity:   quantity,
		Unit:       unit,
		CreatedAt:  time.Now(),
	})
}

func (o *DefaultOrchestrator) recordAudit(req RunRequest, responseID, action, resourceType, resourceID string, metadata map[string]any) {
	if o.audit == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["response_id"] = responseID
	_ = o.audit.Create(context.Background(), &domain.AuditLog{
		ID:           id.New(id.PrefixAudit),
		OrgID:        req.OrgID,
		ProjectID:    req.ProjectID,
		ActorUserID:  req.UserID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     metadata,
		CreatedAt:    time.Now(),
	})
}

func (o *DefaultOrchestrator) startSpan(ctx context.Context, name, typ, responseID, orgID, projectID string) *observability.Span {
	if o.tracer == nil {
		return nil
	}
	return o.tracer.StartSpan(ctx, name,
		observability.WithType(typ),
		observability.WithResponseID(responseID),
		observability.WithTenant(orgID, projectID),
	)
}

func messageText(items []domain.MessageItem) string {
	var b strings.Builder
	for _, item := range items {
		if item.Type != domain.ItemTypeText {
			continue
		}
		if v, ok := item.Content["text"].(string); ok {
			b.WriteString(v)
		}
	}
	return b.String()
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func summarizeResult(s string) string {
	if len(s) <= 512 {
		return s
	}
	return s[:512]
}

func toolErrCode(err error) errcode.Code {
	if e, ok := errcode.As(err); ok {
		return e.Code
	}
	if strings.Contains(err.Error(), "timed out") {
		return errcode.CodeToolTimeout
	}
	if strings.Contains(err.Error(), "not found") {
		return errcode.CodeToolForbidden
	}
	return errcode.CodeToolParamInvalid
}

func normalizeModelErr(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errcode.As(err); ok {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errcode.Wrap(err, errcode.CodeModelTimeout, "model request timed out")
	}
	return errcode.Wrap(err, errcode.CodeModelProviderUnavailable, "model provider unavailable")
}

func userFacingError(err error) string {
	if ec, ok := errcode.As(err); ok {
		return ec.Message
	}
	return err.Error()
}
