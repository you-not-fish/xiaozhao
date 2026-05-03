// Package errcode defines the standard error code taxonomy used across the
// service. Error codes are stable string identifiers such as
// "auth_invalid_credentials"; every error carries an HTTP status hint, a
// user-visible message and an optional wrapped cause. Handlers should never
// return raw errors to the client: they should wrap domain errors via New or
// Wrap, then let the response layer translate to JSON.
package errcode

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	// Generic
	CodeInternal        Code = "internal_error"
	CodeInvalidArgument Code = "invalid_argument"
	CodeNotFound        Code = "not_found"
	CodeConflict        Code = "conflict"
	CodeUnauthorized    Code = "unauthorized"
	CodeForbidden       Code = "forbidden"
	CodeRateLimited     Code = "rate_limited"
	CodeUnavailable     Code = "service_unavailable"
	CodeFeatureDisabled Code = "feature_not_enabled"

	// Auth
	CodeAuthInvalidCredentials Code = "auth_invalid_credentials"
	CodeAuthTokenInvalid       Code = "auth_token_invalid"
	CodeAuthTokenExpired       Code = "auth_token_expired"
	CodeAuthEmailTaken         Code = "auth_email_taken"
	CodeAuthPasswordWeak       Code = "auth_password_weak"

	// Tenant
	CodeOrgNotFound       Code = "org_not_found"
	CodeOrgNotMember      Code = "org_not_member"
	CodeProjectNotFound   Code = "project_not_found"
	CodeInsufficientRole  Code = "insufficient_role"

	// Model / Tool (future-facing, kept here to avoid sprawl)
	CodeModelTimeout       Code = "model_timeout"
	CodeModelRateLimited   Code = "model_rate_limited"
	CodeModelAuthFailed    Code = "model_auth_failed"
	CodeModelContextExceed Code = "model_context_exceeded"
	CodeToolParamInvalid   Code = "tool_param_invalid"
	CodeToolTimeout        Code = "tool_timeout"
	CodeToolForbidden      Code = "tool_forbidden"
)

// Error is the structured application error returned up the call stack.
// Handlers map Error -> JSON via the response layer.
type Error struct {
	Code     Code   // stable machine code
	Message  string // user-visible
	HTTP     int    // http status hint
	Cause    error  // wrapped cause (never serialized)
	Metadata map[string]any
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// New builds a new error.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message, HTTP: httpFor(code)}
}

// Newf builds a new error with printf-style message.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), HTTP: httpFor(code)}
}

// Wrap wraps an existing error with the given code and message. If err is
// already an *Error its code and http status are preserved unless override
// semantics are explicitly desired by the caller.
func Wrap(err error, code Code, message string) *Error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Message: message, HTTP: httpFor(code), Cause: err}
}

// WithMetadata attaches key/value metadata to the error.
func (e *Error) WithMetadata(k string, v any) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any, 2)
	}
	e.Metadata[k] = v
	return e
}

// As tries to cast err to *Error.
func As(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// CodeOf returns the error code, or CodeInternal if err is not an *Error.
func CodeOf(err error) Code {
	if e, ok := As(err); ok {
		return e.Code
	}
	return CodeInternal
}

// HTTPStatus returns the http status hint, or 500.
func HTTPStatus(err error) int {
	if e, ok := As(err); ok && e.HTTP != 0 {
		return e.HTTP
	}
	return http.StatusInternalServerError
}

func httpFor(code Code) int {
	switch code {
	case CodeInvalidArgument, CodeAuthPasswordWeak, CodeToolParamInvalid:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeAuthInvalidCredentials, CodeAuthTokenInvalid, CodeAuthTokenExpired:
		return http.StatusUnauthorized
	case CodeForbidden, CodeInsufficientRole, CodeOrgNotMember, CodeToolForbidden:
		return http.StatusForbidden
	case CodeNotFound, CodeOrgNotFound, CodeProjectNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeAuthEmailTaken:
		return http.StatusConflict
	case CodeRateLimited, CodeModelRateLimited:
		return http.StatusTooManyRequests
	case CodeUnavailable, CodeModelTimeout, CodeToolTimeout:
		return http.StatusServiceUnavailable
	case CodeFeatureDisabled:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}
