package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/xiaozhao/xiaozhao/internal/app/agent"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
)

func TestResponseCreateRequiresAuth(t *testing.T) {
	h := NewResponseHandler(fakeOrchestrator{}, fakeResponseEventRepo{})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"project_id":"proj_1","input":"hi"}`))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestResponseCreateRequiresTenant(t *testing.T) {
	h := NewResponseHandler(fakeOrchestrator{}, fakeResponseEventRepo{})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"project_id":"proj_1","input":"hi"}`))
	req = req.WithContext(httpctx.WithUserID(req.Context(), "user_1"))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestResponseCreateWritesSSE(t *testing.T) {
	h := NewResponseHandler(fakeOrchestrator{}, fakeResponseEventRepo{})
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"project_id":"proj_1","input":"hi"}`))
	ctx := httpctx.WithUserID(req.Context(), "user_1")
	ctx = httpctx.WithOrgID(ctx, "org_1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	if !strings.Contains(rr.Body.String(), "event: response.created") {
		t.Fatalf("SSE body missing response.created: %s", rr.Body.String())
	}
}

func TestResponseEventsReplayChecksTenant(t *testing.T) {
	repo := fakeResponseEventRepo{events: []domain.ResponseEvent{
		{ID: "evt_1", ResponseID: "resp_1", ConversationID: "conv_1", OrgID: "org_1", ProjectID: "proj_1", Seq: 1, Type: domain.EventResponseCreated, Data: []byte(`{"ok":true}`), CreatedAt: time.Now()},
		{ID: "evt_2", ResponseID: "resp_1", ConversationID: "conv_1", OrgID: "org_1", ProjectID: "proj_1", Seq: 2, Type: domain.EventResponseCompleted, Data: []byte(`{"done":true}`), CreatedAt: time.Now()},
	}}
	h := NewResponseHandler(fakeOrchestrator{}, repo)
	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_1/events", nil)
	req = withURLParam(req, "responseID", "resp_1")
	req = req.WithContext(httpctx.WithOrgID(req.Context(), "org_1"))
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Data struct {
			Items []struct {
				Seq int `json:"seq"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(rr.Body.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Data.Items) != 2 || body.Data.Items[0].Seq != 1 || body.Data.Items[1].Seq != 2 {
		t.Fatalf("events not in expected order: %+v", body.Data.Items)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/responses/resp_1/events", nil)
	req = withURLParam(req, "responseID", "resp_1")
	req = req.WithContext(httpctx.WithOrgID(req.Context(), "other_org"))
	rr = httptest.NewRecorder()
	h.Events(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant status = %d, want 404", rr.Code)
	}
}

func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

type fakeOrchestrator struct{}

func (fakeOrchestrator) Run(_ context.Context, req agent.RunRequest) (*agent.RunResult, error) {
	ch := make(chan agent.EventEnvelope, 2)
	ch <- agent.EventEnvelope{Event: &domain.ResponseEvent{
		ID:             "evt_1",
		ResponseID:     "resp_1",
		ConversationID: "conv_1",
		OrgID:          req.OrgID,
		ProjectID:      req.ProjectID,
		Seq:            1,
		Type:           domain.EventResponseCreated,
		Data:           []byte(`{"response_id":"resp_1"}`),
		CreatedAt:      time.Now(),
	}}
	ch <- agent.EventEnvelope{Final: true}
	close(ch)
	return &agent.RunResult{ResponseID: "resp_1", ConversationID: "conv_1", MessageID: "msg_1", Events: ch}, nil
}

type fakeResponseEventRepo struct {
	events []domain.ResponseEvent
}

func (fakeResponseEventRepo) Append(context.Context, *domain.ResponseEvent) error { return nil }
func (r fakeResponseEventRepo) ListByResponse(_ context.Context, responseID string) ([]domain.ResponseEvent, error) {
	var out []domain.ResponseEvent
	for _, e := range r.events {
		if e.ResponseID == responseID {
			out = append(out, e)
		}
	}
	return out, nil
}
