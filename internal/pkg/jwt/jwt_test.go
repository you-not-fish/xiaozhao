package jwt

import (
	"testing"
	"time"
)

func TestIssueParseRoundTrip(t *testing.T) {
	m := NewManager("a-very-long-testing-secret-string", "xiaozhao", time.Minute)
	tok, exp, err := m.Issue("user_abc", "org_xyz")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if tok == "" || exp.Before(time.Now()) {
		t.Fatalf("invalid token/expiry: tok=%q exp=%s", tok, exp)
	}
	claims, err := m.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != "user_abc" || claims.OrgID != "org_xyz" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	m1 := NewManager("secret-one-long-enough-for-hmac", "xiaozhao", time.Minute)
	m2 := NewManager("secret-two-long-enough-for-hmac", "xiaozhao", time.Minute)
	tok, _, err := m1.Issue("user_abc", "")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := m2.Parse(tok); err == nil {
		t.Fatal("expected parse to reject token signed by another key")
	}
}

func TestParseRejectsExpired(t *testing.T) {
	m := NewManager("secret-enough-long-for-hmac-alg", "xiaozhao", -time.Second)
	tok, _, _ := m.Issue("u", "")
	if _, err := m.Parse(tok); err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}
