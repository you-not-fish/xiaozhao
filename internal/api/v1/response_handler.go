package v1

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/xiaozhao/xiaozhao/internal/api/dto"
	"github.com/xiaozhao/xiaozhao/internal/api/response"
	"github.com/xiaozhao/xiaozhao/internal/app/agent"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type ResponseHandler struct {
	orchestrator agent.Orchestrator
	events       domain.ResponseEventRepository
}

func NewResponseHandler(orchestrator agent.Orchestrator, events domain.ResponseEventRepository) *ResponseHandler {
	return &ResponseHandler{orchestrator: orchestrator, events: events}
}

func (h *ResponseHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid, err := httpctx.UserID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
		return
	}
	orgID, err := httpctx.OrgID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "organization context is required"))
		return
	}
	var req dto.CreateResponseRequest
	if err := decode(r, &req); err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnavailable, "streaming is not supported"))
		return
	}

	result, err := h.orchestrator.Run(r.Context(), agent.RunRequest{
		OrgID:                orgID,
		ProjectID:            req.ProjectID,
		UserID:               uid,
		ConversationID:       req.ConversationID,
		Input:                req.Input,
		EnabledTools:         requestedTools(req.Tools),
		ToolKnowledgeBaseIDs: requestedToolKnowledgeBases(req.Tools),
		Model:                req.Model,
		Stream:               true,
	})
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Response-Id", result.ResponseID)
	w.Header().Set("X-Conversation-Id", result.ConversationID)
	w.WriteHeader(http.StatusOK)

	// SSE 只负责传输事件；事件完整性由 Orchestrator 的"先落库后推送"保证。
	for env := range result.Events {
		if env.Event != nil {
			if err := writeSSE(w, env.Event); err != nil {
				return
			}
			flusher.Flush()
		}
		if env.Final {
			return
		}
	}
}

func (h *ResponseHandler) Events(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpctx.OrgID(r.Context())
	if err != nil {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "organization context is required"))
		return
	}
	responseID := chi.URLParam(r, "responseID")
	events, err := h.events.ListByResponse(r.Context(), responseID)
	if err != nil {
		response.WriteError(r.Context(), w, err)
		return
	}
	if len(events) == 0 {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeNotFound, "response not found"))
		return
	}
	// 回放接口不能只凭 response_id 返回数据，必须校验事件所属租户。
	if events[0].OrgID != orgID {
		response.WriteError(r.Context(), w, errcode.New(errcode.CodeNotFound, "response not found"))
		return
	}
	items := make([]dto.ResponseEventResponse, 0, len(events))
	for _, e := range events {
		items = append(items, dto.FromResponseEvent(e))
	}
	response.WriteOK(r.Context(), w, dto.ResponseEventListResponse{Items: items})
}

func requestedTools(tools []dto.ResponseToolRequest) []string {
	if len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if t.Type != "" {
			out = append(out, t.Type)
		}
	}
	return out
}

func requestedToolKnowledgeBases(tools []dto.ResponseToolRequest) map[string][]string {
	out := map[string][]string{}
	for _, t := range tools {
		if t.Type == "" || len(t.KnowledgeBaseIDs) == 0 {
			continue
		}
		out[t.Type] = append(out[t.Type], t.KnowledgeBaseIDs...)
	}
	return out
}

func writeSSE(w http.ResponseWriter, e *domain.ResponseEvent) error {
	data, err := json.Marshal(dto.FromResponseEvent(*e))
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: " + string(e.Type) + "\n")); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
		return err
	}
	return nil
}
