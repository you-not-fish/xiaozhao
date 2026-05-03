package middleware

import (
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/xiaozhao/xiaozhao/internal/api/response"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/logger"
)

// Recovery converts panics into a 500 JSON response and logs a stack trace.
// It is the outermost middleware so it can catch panics from anything above
// it in the chain, including auth and handlers.
func Recovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.From(r.Context()).Error("http.panic",
						zap.Any("panic", rec),
						zap.ByteString("stack", debug.Stack()),
					)
					response.WriteError(r.Context(), w, errcode.New(errcode.CodeInternal, "internal server error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
