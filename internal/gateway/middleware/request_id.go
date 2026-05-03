// Package middleware contains reusable HTTP middlewares: request id
// assignment, access logging, panic recovery, JWT auth, tenant resolution
// and Redis-backed rate limiting. Each middleware is a plain
// func(next http.Handler) http.Handler so Chi's Use() can pick them up.
package middleware

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
)

const headerRequestID = "X-Request-Id"

// RequestID sets an x-request-id header (both ingoing and outgoing) and
// stores the value in the request context so downstream logs can correlate.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get(headerRequestID)
			if rid == "" {
				rid = "req_" + uuid.NewString()
			}
			w.Header().Set(headerRequestID, rid)
			ctx := httpctx.WithRequestID(r.Context(), rid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
