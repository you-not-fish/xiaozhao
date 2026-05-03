// Package httpctx centralises the context keys used to pass per-request
// metadata through the HTTP stack. Keeping them in one place means handlers
// and middlewares never disagree about key types or names.
package httpctx

import (
	"context"
	"errors"
)

type ctxKey string

const (
	keyRequestID ctxKey = "request_id"
	keyUserID    ctxKey = "user_id"
	keyOrgID     ctxKey = "org_id"
	keyTokenOrg  ctxKey = "token_org_id"
)

// WithRequestID stores the request id in ctx.
func WithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, keyRequestID, rid)
}

// RequestID returns the request id from ctx (empty when missing).
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(keyRequestID).(string); ok {
		return v
	}
	return ""
}

// WithUserID stores the authenticated user id in ctx.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyUserID, id)
}

// UserID returns the authenticated user id from ctx. Returns an error when
// missing, so handlers that require auth can return 401 rather than silently
// operate as an anonymous user.
func UserID(ctx context.Context) (string, error) {
	if v, ok := ctx.Value(keyUserID).(string); ok && v != "" {
		return v, nil
	}
	return "", errors.New("httpctx: user id missing from context")
}

// WithOrgID stores the resolved active org id in ctx.
func WithOrgID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyOrgID, id)
}

// OrgID returns the resolved active org id from ctx.
func OrgID(ctx context.Context) (string, error) {
	if v, ok := ctx.Value(keyOrgID).(string); ok && v != "" {
		return v, nil
	}
	return "", errors.New("httpctx: org id missing from context")
}

// WithTokenOrgID stores the org id embedded in the JWT at auth time.
// This is distinct from the active org id resolved by the tenant middleware,
// which may be overridden by a request header.
func WithTokenOrgID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTokenOrg, id)
}

// TokenOrgID returns the JWT-embedded org id from ctx.
func TokenOrgID(ctx context.Context) string {
	if v, ok := ctx.Value(keyTokenOrg).(string); ok {
		return v
	}
	return ""
}
