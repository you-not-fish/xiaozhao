package knowledge

import (
	"strings"
	"testing"

	"github.com/xiaozhao/xiaozhao/internal/infra/parser"
)

func TestChunkerSplitsChineseWithOverlapAndMetadata(t *testing.T) {
	chunker := NewChunker(18, 4)
	parsed := &parser.Document{Parts: []parser.Part{{Text: "第一段介绍产品能力。第二段说明退款政策，包含很多细节。第三段描述支持范围。", Page: 2, Title: "政策"}}}

	chunks := chunker.Split(ChunkInput{
		OrgID: "org_1", ProjectID: "proj_1", KnowledgeBaseID: "kb_1", DocumentID: "doc_1", FileID: "file_1", Parsed: parsed,
	})

	if len(chunks) < 2 {
		t.Fatalf("chunk count = %d, want >= 2", len(chunks))
	}
	if chunks[0].ChunkIndex != 0 || chunks[1].ChunkIndex != 1 {
		t.Fatalf("chunk indexes = %d/%d", chunks[0].ChunkIndex, chunks[1].ChunkIndex)
	}
	if chunks[0].Metadata["page"] != 2 || chunks[0].Metadata["title_path"] != "政策" {
		t.Fatalf("metadata = %#v", chunks[0].Metadata)
	}
	if !strings.Contains(chunks[0].Content, "。") {
		t.Fatalf("first chunk did not split on Chinese punctuation: %q", chunks[0].Content)
	}
}
