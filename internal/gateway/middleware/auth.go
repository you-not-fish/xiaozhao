package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/xiaozhao/xiaozhao/internal/api/response"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
	"github.com/xiaozhao/xiaozhao/internal/pkg/jwt"
)

// Auth rejects requests without a valid bearer token and attaches user/org
// info to the request context. Do not apply to public routes (health check,
// register, login) — put them outside this middleware chain.
func Auth(mgr *jwt.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if authz == "" {
				response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "missing authorization header"))
				return
			}
			parts := strings.SplitN(authz, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "malformed authorization header"))
				return
			}
			claims, err := mgr.Parse(parts[1])
			if err != nil {
				code := errcode.CodeAuthTokenInvalid
				msg := "invalid token"
				if errors.Is(err, jwt.ErrExpired) {
					code = errcode.CodeAuthTokenExpired
					msg = "token expired"
				}
				response.WriteError(r.Context(), w, errcode.New(code, msg))
				return
			}
			ctx := httpctx.WithUserID(r.Context(), claims.UserID)
			if claims.OrgID != "" {
				ctx = httpctx.WithTokenOrgID(ctx, claims.OrgID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
