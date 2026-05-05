package websearch

import (
	"context"
	"strings"
	"time"
)

type MockProvider struct {
	maxResults       int
	defaultFreshness string
}

func NewMockProvider(maxResults int, defaultFreshness string) *MockProvider {
	return &MockProvider{maxResults: maxResults, defaultFreshness: defaultFreshness}
}

func (p *MockProvider) Name() string { return "mock" }

func (p *MockProvider) Search(_ context.Context, in SearchRequest) (*SearchResponse, error) {
	in = NormalizeRequest(in, p.maxResults, p.defaultFreshness)
	items := []Result{
		{
			Title:       "小招 AI Agent 联网搜索示例",
			URL:         "https://example.com/xiaozhao-agent-search",
			SiteName:    "Example",
			Snippet:     "这是 mock 搜索结果，用于本地开发和测试，不会访问真实公网。",
			Summary:     "mock provider 会返回结构化标题、URL、摘要和引用字段，保证 Agent 工具链路可离线跑通。",
			PublishedAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			Score:       0.91,
		},
		{
			Title:       "成熟 Agent 产品的联网搜索实践",
			URL:         "https://example.com/agent-web-search-practice",
			SiteName:    "Example",
			Snippet:     "联网搜索通常作为受控工具执行，并把引用来源返回给最终用户。",
			Summary:     "工具调用应记录参数、结果来源、耗时和错误；回答使用搜索内容时必须展示可点击引用。",
			PublishedAt: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
			Score:       0.84,
		},
	}
	if strings.Contains(strings.ToLower(in.Query), "新闻") || strings.Contains(in.Query, "最新") {
		items[0].Title = "最新资讯 mock 搜索结果"
		items[0].Summary = "这是面向最新/新闻类问题的 mock 搜索结果。"
	}
	if len(items) > in.Count {
		items = items[:in.Count]
	}
	return &SearchResponse{
		Query:       in.Query,
		Provider:    p.Name(),
		Results:     items,
		RetrievedAt: time.Now(),
	}, nil
}
