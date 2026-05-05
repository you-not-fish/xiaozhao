package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/app/tool"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/infra/model"
	"github.com/xiaozhao/xiaozhao/internal/infra/websearch"
	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
	"github.com/xiaozhao/xiaozhao/internal/pkg/id"
)

func TestOrchestratorNoToolCompletes(t *testing.T) {
	fx := newAgentFixture(t, model.NewMockProvider(model.MockScript{
		Text:   "hello from model",
		Finish: model.FinishStop,
		Usage:  &model.Usage{InputTokens: 3, OutputTokens: 4},
	}))

	res, err := fx.orch.Run(context.Background(), baseRunRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	events, finalErr := drainEvents(res.Events)
	if finalErr != nil {
		t.Fatalf("final error = %v", finalErr)
	}
	assertEventTypes(t, events, domain.EventResponseCreated, domain.EventMessageDelta, domain.EventMessageCompleted, domain.EventUsageUpdated, domain.EventResponseCompleted)
	if got := fx.messages.status[res.MessageID]; got != domain.MessageStatusCompleted {
		t.Fatalf("assistant status = %s, want completed", got)
	}
}

func TestOrchestratorToolLoopCompletes(t *testing.T) {
	fx := newAgentFixture(t, model.NewMockProvider(
		model.MockScript{
			Text: "查一下时间",
			ToolCalls: []model.ToolCall{{
				ID:        "call_test_1",
				Name:      "current_time",
				Arguments: json.RawMessage(`{}`),
			}},
		},
		model.MockScript{Text: "现在可以回答了", Finish: model.FinishStop},
	))

	res, err := fx.orch.Run(context.Background(), baseRunRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	events, finalErr := drainEvents(res.Events)
	if finalErr != nil {
		t.Fatalf("final error = %v", finalErr)
	}
	assertEventTypes(t, events, domain.EventToolCallCreated, domain.EventToolCallCompleted, domain.EventResponseCompleted)
	if len(fx.toolCalls.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(fx.toolCalls.calls))
	}
	if fx.toolCalls.calls[0].Status != domain.ToolCallStatusSucceeded {
		t.Fatalf("tool status = %s, want succeeded", fx.toolCalls.calls[0].Status)
	}
}

func TestOrchestratorOverridesForgedKnowledgeSearchScope(t *testing.T) {
	fx := newAgentFixture(t, model.NewMockProvider(
		model.MockScript{
			ToolCalls: []model.ToolCall{{
				ID:        "call_knowledge",
				Name:      "knowledge_search",
				Arguments: json.RawMessage(`{"query":"退款","org_id":"evil_org","project_id":"evil_proj","user_id":"evil_user","allowed_knowledge_base_ids":["kb_evil"]}`),
			}},
		},
		model.MockScript{Text: "done", Finish: model.FinishStop},
	))
	capture := &captureArgsTool{name: "knowledge_search"}
	if err := fx.tools.Register(capture); err != nil {
		t.Fatalf("register knowledge_search: %v", err)
	}
	req := baseRunRequest()
	req.ToolKnowledgeBaseIDs = map[string][]string{"knowledge_search": {"kb_1"}}

	res, err := fx.orch.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	_, finalErr := drainEvents(res.Events)
	if finalErr != nil {
		t.Fatalf("final error = %v", finalErr)
	}
	got := capture.args
	if got["org_id"] != "org_1" || got["project_id"] != "proj_1" || got["user_id"] != "user_1" {
		t.Fatalf("tenant args not overridden: %#v", got)
	}
	allowed, ok := got["allowed_knowledge_base_ids"].([]any)
	if !ok || len(allowed) != 1 || allowed[0] != "kb_1" {
		t.Fatalf("allowed KBs = %#v", got["allowed_knowledge_base_ids"])
	}
}

func TestOrchestratorEmitsCitationFromWebSearch(t *testing.T) {
	fx := newAgentFixture(t, model.NewMockProvider(
		model.MockScript{
			ToolCalls: []model.ToolCall{{
				ID:        "call_web",
				Name:      "web_search",
				Arguments: json.RawMessage(`{"query":"最新 AI 新闻","count":1}`),
			}},
		},
		model.MockScript{Text: "done", Finish: model.FinishStop},
	))
	if err := fx.tools.Register(tool.NewWebSearch(websearch.NewMockProvider(5, "noLimit"), 5, "noLimit", 15)); err != nil {
		t.Fatalf("register web_search: %v", err)
	}
	res, err := fx.orch.Run(context.Background(), baseRunRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	events, finalErr := drainEvents(res.Events)
	if finalErr != nil {
		t.Fatalf("final error = %v", finalErr)
	}
	assertEventTypes(t, events, domain.EventCitationAdded, domain.EventToolCallCompleted, domain.EventResponseCompleted)
	if !hasMessageItemType(t, fx.messages, res.MessageID, domain.ItemTypeCitation) {
		t.Fatal("citation message item not persisted")
	}
}

func TestOrchestratorMissingToolFails(t *testing.T) {
	fx := newAgentFixture(t, model.NewMockProvider(model.MockScript{
		ToolCalls: []model.ToolCall{{
			ID:        "call_missing",
			Name:      "missing_tool",
			Arguments: json.RawMessage(`{}`),
		}},
	}))

	res, err := fx.orch.Run(context.Background(), baseRunRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	events, finalErr := drainEvents(res.Events)
	if finalErr == nil {
		t.Fatal("final error = nil, want error")
	}
	assertEventTypes(t, events, domain.EventToolCallCreated, domain.EventToolCallFailed, domain.EventResponseFailed)
	if got := fx.messages.status[res.MessageID]; got != domain.MessageStatusFailed {
		t.Fatalf("assistant status = %s, want failed", got)
	}
}

func TestOrchestratorTruncatesLargeToolResult(t *testing.T) {
	fx := newAgentFixture(t, model.NewMockProvider(
		model.MockScript{
			ToolCalls: []model.ToolCall{{
				ID:        "call_big",
				Name:      "big_result",
				Arguments: json.RawMessage(`{}`),
			}},
		},
		model.MockScript{Text: "done", Finish: model.FinishStop},
	))
	if err := fx.tools.Register(bigResultTool{}); err != nil {
		t.Fatalf("register big tool: %v", err)
	}

	res, err := fx.orch.Run(context.Background(), baseRunRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	events, finalErr := drainEvents(res.Events)
	if finalErr != nil {
		t.Fatalf("final error = %v", finalErr)
	}
	completed := findEvent(events, domain.EventToolCallCompleted)
	if completed == nil {
		t.Fatal("tool_call.completed not found")
	}
	var data map[string]any
	if err := json.Unmarshal(completed.Data, &data); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if data["truncated"] != true {
		t.Fatalf("truncated = %v, want true", data["truncated"])
	}
}

func TestOrchestratorModelFailureWritesFailedEvent(t *testing.T) {
	fx := newAgentFixture(t, failingProvider{})
	res, err := fx.orch.Run(context.Background(), baseRunRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	events, finalErr := drainEvents(res.Events)
	if finalErr == nil {
		t.Fatal("final error = nil, want error")
	}
	assertEventTypes(t, events, domain.EventResponseCreated, domain.EventResponseFailed)
	if got := fx.messages.status[res.MessageID]; got != domain.MessageStatusFailed {
		t.Fatalf("assistant status = %s, want failed", got)
	}
}

func baseRunRequest() RunRequest {
	return RunRequest{
		OrgID:     "org_1",
		ProjectID: "proj_1",
		UserID:    "user_1",
		Input:     "hello",
		Stream:    true,
	}
}

type agentFixture struct {
	orch      *DefaultOrchestrator
	tools     *tool.Registry
	messages  *fakeMessageRepo
	toolCalls *fakeToolCallRepo
}

func newAgentFixture(t *testing.T, provider model.Provider) *agentFixture {
	t.Helper()
	convs := &fakeConversationRepo{items: map[string]*domain.Conversation{}}
	projects := fakeProjectRepo{p: &domain.Project{ID: "proj_1", OrgID: "org_1", Name: "p"}}
	members := fakeMemberRepo{m: &domain.OrgMember{OrgID: "org_1", UserID: "user_1", Role: domain.RoleOwner, Status: domain.MemberStatusActive}}
	messages := newFakeMessageRepo()
	events := &fakeEventRepo{}
	tools := tool.NewRegistry()
	if err := tools.Register(tool.NewCurrentTime()); err != nil {
		t.Fatalf("register current_time: %v", err)
	}
	if err := tools.Register(tool.NewCalculator()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	toolCalls := &fakeToolCallRepo{}
	convSvc := NewConversationService(convs, projects, rbac.NewChecker(members))
	orch := NewDefaultOrchestrator(OrchestratorDeps{
		Conversations: convSvc,
		Messages:      messages,
		Events:        events,
		ModelProvider: provider,
		ModelName:     "mock",
		Tools:         tools,
		ModelCalls:    &fakeModelInvocationRepo{},
		ToolCalls:     toolCalls,
		Usage:         &fakeUsageRepo{},
		Config:        config.AgentConfig{MaxToolIterations: 5, ModelTimeoutSec: 5, ToolTimeoutSec: 1, MaxToolResultKB: 1},
	})
	return &agentFixture{orch: orch, tools: tools, messages: messages, toolCalls: toolCalls}
}

func drainEvents(ch <-chan EventEnvelope) ([]domain.ResponseEvent, error) {
	var out []domain.ResponseEvent
	var finalErr error
	for env := range ch {
		if env.Event != nil {
			out = append(out, *env.Event)
		}
		if env.Err != nil {
			finalErr = env.Err
		}
	}
	return out, finalErr
}

func assertEventTypes(t *testing.T, events []domain.ResponseEvent, wants ...domain.EventType) {
	t.Helper()
	got := make([]domain.EventType, 0, len(events))
	for _, e := range events {
		got = append(got, e.Type)
	}
	for _, want := range wants {
		if !slices.Contains(got, want) {
			t.Fatalf("event %s not found in %v", want, got)
		}
	}
}

func findEvent(events []domain.ResponseEvent, typ domain.EventType) *domain.ResponseEvent {
	for i := range events {
		if events[i].Type == typ {
			return &events[i]
		}
	}
	return nil
}

func hasMessageItemType(t *testing.T, repo *fakeMessageRepo, messageID string, typ domain.MessageItemType) bool {
	t.Helper()
	items, err := repo.ListItemsByMessage(context.Background(), messageID)
	if err != nil {
		t.Fatalf("ListItemsByMessage() error = %v", err)
	}
	for _, item := range items {
		if item.Type == typ {
			return true
		}
	}
	return false
}

type bigResultTool struct{}

func (bigResultTool) Spec() tool.Spec {
	return tool.Spec{
		Name:         "big_result",
		Description:  "return a large payload",
		InputSchema:  json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(`{"type":"object"}`),
		TimeoutMS:    1000,
		ProviderType: "builtin",
	}
}

func (bigResultTool) Execute(context.Context, json.RawMessage) (any, error) {
	return map[string]any{"text": strings.Repeat("x", 2048)}, nil
}

type captureArgsTool struct {
	name string
	args map[string]any
}

func (t *captureArgsTool) Spec() tool.Spec {
	return tool.Spec{
		Name:         t.name,
		Description:  "capture args",
		InputSchema:  json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(`{"type":"object"}`),
		TimeoutMS:    1000,
		ProviderType: "builtin",
	}
}

func (t *captureArgsTool) Execute(_ context.Context, args json.RawMessage) (any, error) {
	_ = json.Unmarshal(args, &t.args)
	return map[string]any{"ok": true}, nil
}

type failingProvider struct{}

func (failingProvider) Name() string { return "failing" }
func (failingProvider) Chat(context.Context, model.ChatRequest) (model.Stream, error) {
	return nil, errors.New("boom")
}

type fakeConversationRepo struct {
	items map[string]*domain.Conversation
}

func (r *fakeConversationRepo) Create(_ context.Context, c *domain.Conversation) error {
	cp := *c
	r.items[c.ID] = &cp
	return nil
}
func (r *fakeConversationRepo) GetByID(_ context.Context, id string) (*domain.Conversation, error) {
	if c := r.items[id]; c != nil {
		cp := *c
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (r *fakeConversationRepo) ListByProject(context.Context, string, string, int) ([]domain.Conversation, error) {
	return nil, nil
}
func (r *fakeConversationRepo) Update(_ context.Context, c *domain.Conversation) error {
	cp := *c
	r.items[c.ID] = &cp
	return nil
}
func (r *fakeConversationRepo) Delete(context.Context, string) error { return nil }

type fakeProjectRepo struct{ p *domain.Project }

func (r fakeProjectRepo) Create(context.Context, *domain.Project) error { return nil }
func (r fakeProjectRepo) GetByID(_ context.Context, id string) (*domain.Project, error) {
	if r.p != nil && r.p.ID == id {
		cp := *r.p
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (r fakeProjectRepo) ListByOrg(context.Context, string) ([]domain.Project, error) {
	return nil, nil
}
func (r fakeProjectRepo) Update(context.Context, *domain.Project) error { return nil }
func (r fakeProjectRepo) Delete(context.Context, string) error          { return nil }

type fakeMemberRepo struct{ m *domain.OrgMember }

func (r fakeMemberRepo) Upsert(context.Context, *domain.OrgMember) error { return nil }
func (r fakeMemberRepo) Get(_ context.Context, orgID, userID string) (*domain.OrgMember, error) {
	if r.m != nil && r.m.OrgID == orgID && r.m.UserID == userID {
		cp := *r.m
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (r fakeMemberRepo) ListByOrg(context.Context, string) ([]domain.OrgMember, error) {
	return nil, nil
}
func (r fakeMemberRepo) Remove(context.Context, string, string) error { return nil }

type fakeMessageRepo struct {
	mu       sync.Mutex
	messages []domain.Message
	items    map[string][]domain.MessageItem
	status   map[string]domain.MessageStatus
}

func newFakeMessageRepo() *fakeMessageRepo {
	return &fakeMessageRepo{items: map[string][]domain.MessageItem{}, status: map[string]domain.MessageStatus{}}
}
func (r *fakeMessageRepo) CreateMessage(_ context.Context, m *domain.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *m
	r.messages = append(r.messages, cp)
	r.status[m.ID] = m.Status
	return nil
}
func (r *fakeMessageRepo) UpdateMessageStatus(_ context.Context, id string, status domain.MessageStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status[id] = status
	return nil
}
func (r *fakeMessageRepo) AppendItem(_ context.Context, item *domain.MessageItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *item
	r.items[item.MessageID] = append(r.items[item.MessageID], cp)
	return nil
}
func (r *fakeMessageRepo) ListByConversation(_ context.Context, convID string, _ int) ([]domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Message
	for _, m := range r.messages {
		if m.ConversationID == convID {
			out = append(out, m)
		}
	}
	return out, nil
}
func (r *fakeMessageRepo) ListItemsByMessage(_ context.Context, messageID string) ([]domain.MessageItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.MessageItem(nil), r.items[messageID]...), nil
}

type fakeEventRepo struct {
	mu     sync.Mutex
	events []domain.ResponseEvent
}

func (r *fakeEventRepo) Append(_ context.Context, e *domain.ResponseEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *e
	r.events = append(r.events, cp)
	return nil
}
func (r *fakeEventRepo) ListByResponse(_ context.Context, responseID string) ([]domain.ResponseEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.ResponseEvent
	for _, e := range r.events {
		if e.ResponseID == responseID {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeModelInvocationRepo struct{}

func (*fakeModelInvocationRepo) Create(context.Context, *domain.ModelInvocation) error { return nil }
func (*fakeModelInvocationRepo) ListByOrg(context.Context, string, int) ([]domain.ModelInvocation, error) {
	return nil, nil
}
func (*fakeModelInvocationRepo) ListByOrgProject(context.Context, string, string, int) ([]domain.ModelInvocation, error) {
	return nil, nil
}

type fakeToolCallRepo struct {
	mu    sync.Mutex
	calls []domain.ToolCall
}

func (r *fakeToolCallRepo) Create(_ context.Context, c *domain.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *c
	r.calls = append(r.calls, cp)
	return nil
}
func (r *fakeToolCallRepo) Update(_ context.Context, c *domain.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.calls {
		if r.calls[i].ID == c.ID {
			r.calls[i] = *c
			return nil
		}
	}
	return nil
}
func (*fakeToolCallRepo) SaveResult(context.Context, *domain.ToolResult) error { return nil }
func (*fakeToolCallRepo) ListByResponse(context.Context, string) ([]domain.ToolCall, error) {
	return nil, nil
}
func (*fakeToolCallRepo) ListByOrg(context.Context, string, string, int) ([]domain.ToolCall, error) {
	return nil, nil
}

type fakeUsageRepo struct{}

func (*fakeUsageRepo) Create(context.Context, *domain.UsageRecord) error { return nil }
func (*fakeUsageRepo) ListByOrg(context.Context, string, int) ([]domain.UsageRecord, error) {
	return nil, nil
}

var _ = time.Now
var _ = id.New
