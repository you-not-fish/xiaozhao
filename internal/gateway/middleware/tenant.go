package middleware

import (
	"context"
	"net/http"

	"github.com/xiaozhao/xiaozhao/internal/api/response"
	"github.com/xiaozhao/xiaozhao/internal/domain"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

// MembershipChecker is the minimal interface Tenant needs — it matches the
// rbac.Checker but is kept as an interface so we don't create a cyclic
// dependency between middleware and the app layer.
type MembershipChecker interface {
	Require(ctx context.Context, orgID, userID string, min domain.Role) (*domain.OrgMember, error)
}

// Tenant resolves the active organization for the request. Resolution order:
//   1. X-Organization-Id header
//   2. org_id embedded in the JWT
//
// The middleware rejects the request if neither is present, or if the user
// isn't an active member of the resolved org. Handlers that don't need a
// tenant (e.g. /v1/orgs which lists tenants for a user) should mount outside
// this middleware.
func Tenant(checker MembershipChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := httpctx.UserID(r.Context())
			if err != nil {
				response.WriteError(r.Context(), w, errcode.New(errcode.CodeUnauthorized, "authentication required"))
				return
			}
			orgID := r.Header.Get("X-Organization-Id")
			if orgID == "" {
				orgID = httpctx.TokenOrgID(r.Context())
			}
			if orgID == "" {
				response.WriteError(r.Context(), w, errcode.New(errcode.CodeInvalidArgument, "organization context is required"))
				return
			}
			if _, err := checker.Require(r.Context(), orgID, userID, domain.RoleViewer); err != nil {
				response.WriteError(r.Context(), w, err)
				return
			}
			ctx := httpctx.WithOrgID(r.Context(), orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
