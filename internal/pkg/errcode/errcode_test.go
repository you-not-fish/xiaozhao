package errcode

import (
	"errors"
	"net/http"
	"testing"
)

func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code   Code
		status int
	}{
		{CodeInvalidArgument, http.StatusBadRequest},
		{CodeUnauthorized, http.StatusUnauthorized},
		{CodeAuthTokenExpired, http.StatusUnauthorized},
		{CodeForbidden, http.StatusForbidden},
		{CodeOrgNotMember, http.StatusForbidden},
		{CodeNotFound, http.StatusNotFound},
		{CodeAuthEmailTaken, http.StatusConflict},
		{CodeRateLimited, http.StatusTooManyRequests},
		{CodeFeatureDisabled, http.StatusNotImplemented},
		{CodeInternal, http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := HTTPStatus(New(c.code, "x")); got != c.status {
			t.Fatalf("%s -> %d, want %d", c.code, got, c.status)
		}
	}
	// Non-errcode error falls back to 500.
	if got := HTTPStatus(errors.New("raw")); got != http.StatusInternalServerError {
		t.Fatalf("raw error -> %d, want 500", got)
	}
}

func TestUnwrap(t *testing.T) {
	base := errors.New("boom")
	e := Wrap(base, CodeInternal, "wrapped")
	if !errors.Is(e, base) {
		t.Fatal("wrap should preserve wrapped error via Unwrap")
	}
}

func TestAsAndCodeOf(t *testing.T) {
	e := New(CodeNotFound, "missing")
	if got, ok := As(e); !ok || got.Code != CodeNotFound {
		t.Fatalf("As failed: ok=%v code=%s", ok, got.Code)
	}
	if got := CodeOf(e); got != CodeNotFound {
		t.Fatalf("CodeOf = %s", got)
	}
	if got := CodeOf(errors.New("plain")); got != CodeInternal {
		t.Fatalf("CodeOf plain error = %s, want internal_error", got)
	}
}
