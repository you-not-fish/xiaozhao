package id

import (
	"strings"
	"testing"
)

func TestNewHasPrefix(t *testing.T) {
	got := New(PrefixUser)
	if !strings.HasPrefix(got, "user_") {
		t.Fatalf("expected prefix 'user_', got %q", got)
	}
	if !HasPrefix(got, PrefixUser) {
		t.Fatalf("HasPrefix(user) = false for %q", got)
	}
	if HasPrefix(got, PrefixOrg) {
		t.Fatalf("HasPrefix(org) = true for user id %q", got)
	}
}

func TestParse(t *testing.T) {
	raw := New(PrefixOrg)
	p, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p != PrefixOrg {
		t.Fatalf("prefix = %s, want org", p)
	}
}

func TestParseMalformed(t *testing.T) {
	cases := []string{"", "user", "user_", "_123", "user_zzz"}
	for _, c := range cases {
		if _, _, err := Parse(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}
