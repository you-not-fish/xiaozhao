package password

import "testing"

func TestHashVerify(t *testing.T) {
	h, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := Verify(h, "correct horse battery staple"); err != nil {
		t.Fatalf("verify(correct): %v", err)
	}
	if err := Verify(h, "wrong"); err == nil {
		t.Fatal("expected mismatch")
	} else if err != ErrMismatch {
		t.Fatalf("want ErrMismatch, got %v", err)
	}
}

func TestHashEmpty(t *testing.T) {
	if _, err := Hash(""); err == nil {
		t.Fatal("expected error on empty password")
	}
}
