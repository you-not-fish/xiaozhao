package tool

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/infra/websearch"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type WebSearch struct {
	provider         websearch.Provider
	maxResults       int
	defaultFreshness string
	timeoutMS        int
}

func NewWebSearch(provider websearch.Provider, maxResults int, defaultFreshness string, timeoutSeconds int) *WebSearch {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 15
	}
	return &WebSearch{
		provider:         provider,
		maxResults:       maxResults,
		defaultFreshness: defaultFreshness,
		timeoutMS:        timeoutSeconds * 1000,
	}
}

var webSearchInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "需要联网检索的问题或关键词，适合最新、新闻、政策、价格等时效性信息。" },
    "count": { "type": "integer", "minimum": 1, "maximum": 10, "default": 5 },
    "freshness": { "type": "string", "enum": ["noLimit", "oneDay", "oneWeek", "oneMonth", "oneYear"] },
    "include_domains": { "type": "array", "items": { "type": "string" } },
    "exclude_domains": { "type": "array", "items": { "type": "string" } }
  },
  "required": ["query"],
  "additionalProperties": false
}`)

func (t *WebSearch) Spec() Spec {
	return Spec{
		Name:         "web_search",
		Description:  "搜索公网信息并返回带 URL 引用的结果。适合回答最新信息、新闻、政策、价格、公告和事实核验问题。",
		Category:     "web",
		RiskLevel:    RiskL0,
		InputSchema:  webSearchInputSchema,
		OutputSchema: json.RawMessage(`{"type":"object"}`),
		TimeoutMS:    t.timeoutMS,
		ProviderType: "builtin",
	}
}

type webSearchArgs struct {
	Query          string   `json:"query"`
	Count          int      `json:"count"`
	Freshness      string   `json:"freshness"`
	IncludeDomains []string `json:"include_domains"`
	ExcludeDomains []string `json:"exclude_domains"`
}

func (t *WebSearch) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	if t.provider == nil {
		return nil, errcode.New(errcode.CodeUnavailable, "web search provider is unavailable")
	}
	var in webSearchArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, errcode.Wrap(err, errcode.CodeToolParamInvalid, "decode web_search args")
		}
	}
	req := websearch.NormalizeRequest(websearch.SearchRequest{
		Query:          in.Query,
		Count:          in.Count,
		Freshness:      in.Freshness,
		IncludeDomains: in.IncludeDomains,
		ExcludeDomains: in.ExcludeDomains,
	}, t.maxResults, t.defaultFreshness)
	if strings.TrimSpace(req.Query) == "" {
		return nil, errcode.New(errcode.CodeToolParamInvalid, "query is required")
	}
	res, err := t.provider.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(res.Results))
	citations := make([]map[string]any, 0, len(res.Results))
	for i, item := range res.Results {
		citedText := firstSnippet(item.Summary, item.Snippet)
		citation := map[string]any{
			"source_type":  "web",
			"url":          item.URL,
			"title":        item.Title,
			"site_name":    item.SiteName,
			"cited_text":   citedText,
			"published_at": item.PublishedAt,
			"retrieved_at": res.RetrievedAt.Format(time.RFC3339),
			"provider":     res.Provider,
		}
		items = append(items, map[string]any{
			"index":        i,
			"title":        item.Title,
			"url":          item.URL,
			"site_name":    item.SiteName,
			"site_icon":    item.SiteIcon,
			"snippet":      item.Snippet,
			"summary":      item.Summary,
			"published_at": item.PublishedAt,
			"score":        item.Score,
			"citation":     citation,
		})
		citations = append(citations, citation)
	}
	return map[string]any{
		"query":        res.Query,
		"provider":     res.Provider,
		"retrieved_at": res.RetrievedAt.Format(time.RFC3339),
		"items":        items,
		"citations":    citations,
	}, nil
}

func firstSnippet(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
