package tool

import (
	"context"
	"encoding/json"

	knowledgeapp "github.com/xiaozhao/xiaozhao/internal/app/knowledge"
	"github.com/xiaozhao/xiaozhao/internal/domain"
)

type KnowledgeSearcher interface {
	Search(ctx context.Context, in knowledgeapp.SearchInput) ([]domain.KnowledgeSearchResult, error)
}

type KnowledgeSearch struct {
	searcher KnowledgeSearcher
}

func NewKnowledgeSearch(searcher KnowledgeSearcher) *KnowledgeSearch {
	return &KnowledgeSearch{searcher: searcher}
}

var knowledgeSearchInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "要在团队知识库中检索的问题或关键词。" },
    "knowledge_base_ids": {
      "type": "array",
      "items": { "type": "string" },
      "description": "可选。只检索指定知识库。"
    },
    "top_k": { "type": "integer", "minimum": 1, "maximum": 50 }
  },
  "required": ["query"],
  "additionalProperties": true
}`)

// additionalProperties 允许 Orchestrator 在执行前注入 org_id/project_id/user_id
// 和 allowed_knowledge_base_ids；这些字段不暴露给模型决策，且会覆盖模型伪造值。

func (t *KnowledgeSearch) Spec() Spec {
	return Spec{
		Name:         "knowledge_search",
		Description:  "查询当前组织和项目内已入库的团队知识库，返回可引用的相关片段。",
		Category:     "knowledge",
		RiskLevel:    RiskL0,
		InputSchema:  knowledgeSearchInputSchema,
		OutputSchema: json.RawMessage(`{"type":"object"}`),
		TimeoutMS:    15000,
		ProviderType: "builtin",
	}
}

type knowledgeSearchArgs struct {
	Query                   string   `json:"query"`
	KnowledgeBaseIDs        []string `json:"knowledge_base_ids"`
	TopK                    int      `json:"top_k"`
	OrgID                   string   `json:"org_id"`
	ProjectID               string   `json:"project_id"`
	UserID                  string   `json:"user_id"`
	AllowedKnowledgeBaseIDs []string `json:"allowed_knowledge_base_ids"`
}

func (t *KnowledgeSearch) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var in knowledgeSearchArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	kbIDs := intersectKnowledgeIDs(in.KnowledgeBaseIDs, in.AllowedKnowledgeBaseIDs)
	results, err := t.searcher.Search(ctx, knowledgeapp.SearchInput{
		OrgID: in.OrgID, ProjectID: in.ProjectID, UserID: in.UserID,
		KnowledgeBaseIDs: kbIDs, Query: in.Query, TopK: in.TopK,
	})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"chunk_id":          r.ChunkID,
			"document_id":       r.DocumentID,
			"file_id":           r.FileID,
			"filename":          r.Filename,
			"knowledge_base_id": r.KnowledgeBaseID,
			"score":             r.Score,
			"content":           r.Content,
			"citation": map[string]any{
				"file_id":     r.FileID,
				"filename":    r.Filename,
				"document_id": r.DocumentID,
				"chunk_id":    r.ChunkID,
				"metadata":    r.Metadata,
			},
		})
	}
	return map[string]any{"query": in.Query, "items": items}, nil
}

func intersectKnowledgeIDs(requested, allowed []string) []string {
	if len(allowed) == 0 {
		return requested
	}
	allow := map[string]bool{}
	for _, id := range allowed {
		allow[id] = true
	}
	out := make([]string, 0, len(requested))
	if len(requested) == 0 {
		for _, id := range allowed {
			out = append(out, id)
		}
		return out
	}
	for _, id := range requested {
		if allow[id] {
			out = append(out, id)
		}
	}
	return out
}
