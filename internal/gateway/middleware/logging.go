package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/logger"
)

// Logging records the outcome of every HTTP request and attaches the
// per-request logger (with request_id / user_id / org_id fields) to ctx so
// handlers can pick it up via logger.From(ctx).
func Logging(base *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			rid := httpctx.RequestID(r.Context())
			lg := base.With(
				zap.String("request_id", rid),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote", clientIP(r)),
			)
			ctx := logger.Into(r.Context(), lg)
			next.ServeHTTP(ww, r.WithContext(ctx))

			fields := []zap.Field{
				zap.Int("status", ww.status),
				zap.Int("bytes", ww.bytes),
				zap.Duration("elapsed", time.Since(start)),
			}
			if uid, err := httpctx.UserID(ctx); err == nil {
				fields = append(fields, zap.String("user_id", uid))
			}
			if oid, err := httpctx.OrgID(ctx); err == nil {
				fields = append(fields, zap.String("org_id", oid))
			}
			lg.Info("http.access", fields...)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Real-Ip"); v != "" {
		return v
	}
	return r.RemoteAddr
}
