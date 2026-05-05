package embedding

import (
	"context"
	"testing"
)

func TestMockProviderReturnsDeterministicVectors(t *testing.T) {
	p := NewMockProvider("mock", 8)
	a, usage, err := p.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := p.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if usage.InputTokens == 0 {
		t.Fatal("usage input tokens = 0")
	}
	if len(a) != 1 || len(a[0]) != 8 {
		t.Fatalf("vector shape = %d/%d", len(a), len(a[0]))
	}
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("vector not deterministic at %d", i)
		}
	}
}
