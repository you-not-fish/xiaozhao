// Package response writes consistent JSON payloads. Handlers call WriteOK or
// WriteError; never json.NewEncoder directly. This keeps the envelope shape
// ({data, error, request_id}) stable across every endpoint.
package response

import (
	"context"
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/logger"
)

// Envelope is the top-level JSON shape returned by every endpoint.
type Envelope struct {
	Data      any          `json:"data,omitempty"`
	Error     *ErrorObject `json:"error,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
}

// ErrorObject is serialised when an error is being returned.
type ErrorObject struct {
	Code     errcode.Code   `json:"code"`
	Message  string         `json:"message"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// WriteOK serialises data with HTTP 200.
func WriteOK(ctx context.Context, w http.ResponseWriter, data any) {
	Write(ctx, w, http.StatusOK, data)
}

// Write serialises data with a caller-chosen HTTP status.
func Write(ctx context.Context, w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	env := Envelope{Data: data, RequestID: httpctx.RequestID(ctx)}
	if err := json.NewEncoder(w).Encode(env); err != nil {
		logger.From(ctx).Warn("response.encode", zap.Error(err))
	}
}

// WriteError maps err to the envelope. If err is a *errcode.Error we honour
// its HTTP status; otherwise we fall back to 500.
func WriteError(ctx context.Context, w http.ResponseWriter, err error) {
	status := errcode.HTTPStatus(err)
	eo := &ErrorObject{Code: errcode.CodeOf(err), Message: err.Error()}
	if ec, ok := errcode.As(err); ok {
		eo.Code = ec.Code
		eo.Message = ec.Message
		eo.Metadata = ec.Metadata
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	env := Envelope{Error: eo, RequestID: httpctx.RequestID(ctx)}
	if err := json.NewEncoder(w).Encode(env); err != nil {
		logger.From(ctx).Warn("response.encode.error", zap.Error(err))
	}
	// Log 5xx errors at warn/error so they show up even without trace infra.
	if status >= 500 {
		logger.From(ctx).Error("http.error", zap.Int("status", status), zap.Error(err))
	}
}
