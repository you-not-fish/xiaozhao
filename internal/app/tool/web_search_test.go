package tool

import (
	"encoding/json"
	"testing"

	"github.com/xiaozhao/xiaozhao/internal/infra/websearch"
)

func TestWebSearchReturnsCitations(t *testing.T) {
	tl := NewWebSearch(websearch.NewMockProvider(5, "noLimit"), 5, "noLimit", 15)
	res, err := tl.Execute(t.Context(), json.RawMessage(`{"query":"最新 AI 新闻","count":2}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	obj, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", res)
	}
	citations, ok := obj["citations"].([]map[string]any)
	if !ok || len(citations) == 0 {
		t.Fatalf("citations = %#v", obj["citations"])
	}
	if citations[0]["source_type"] != "web" || citations[0]["url"] == "" {
		t.Fatalf("citation = %#v", citations[0])
	}
}
