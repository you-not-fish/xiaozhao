package tool

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/infra/webfetch"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type WebFetch struct {
	fetcher   *webfetch.Fetcher
	timeoutMS int
}

func NewWebFetch(fetcher *webfetch.Fetcher, timeoutSeconds int) *WebFetch {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 15
	}
	return &WebFetch{fetcher: fetcher, timeoutMS: timeoutSeconds * 1000}
}

var webFetchInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "url": { "type": "string", "description": "要打开的 http/https 公网 URL。" },
    "query": { "type": "string", "description": "可选。用于定位页面内相关片段的关键词。" },
    "max_chars": { "type": "integer", "minimum": 500, "maximum": 8000, "default": 4000 }
  },
  "required": ["url"],
  "additionalProperties": false
}`)

func (t *WebFetch) Spec() Spec {
	return Spec{
		Name:         "web_fetch",
		Description:  "打开一个公网网页并抽取正文片段。只能访问 http/https 公网地址，不能访问内网或本机地址。",
		Category:     "web",
		RiskLevel:    RiskL0,
		InputSchema:  webFetchInputSchema,
		OutputSchema: json.RawMessage(`{"type":"object"}`),
		TimeoutMS:    t.timeoutMS,
		ProviderType: "builtin",
	}
}

type webFetchArgs struct {
	URL      string `json:"url"`
	Query    string `json:"query"`
	MaxChars int    `json:"max_chars"`
}

func (t *WebFetch) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	if t.fetcher == nil {
		return nil, errcode.New(errcode.CodeUnavailable, "web fetcher is unavailable")
	}
	var in webFetchArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, errcode.Wrap(err, errcode.CodeToolParamInvalid, "decode web_fetch args")
		}
	}
	if strings.TrimSpace(in.URL) == "" {
		return nil, errcode.New(errcode.CodeToolParamInvalid, "url is required")
	}
	res, err := t.fetcher.Fetch(ctx, webfetch.Request{URL: in.URL, Query: in.Query, MaxChars: in.MaxChars})
	if err != nil {
		return nil, err
	}
	citation := map[string]any{
		"source_type":  "web",
		"url":          res.FinalURL,
		"title":        res.Title,
		"site_name":    "",
		"cited_text":   res.Snippet,
		"published_at": "",
		"retrieved_at": res.RetrievedAt.Format(time.RFC3339),
		"provider":     "web_fetch",
	}
	return map[string]any{
		"url":          res.URL,
		"final_url":    res.FinalURL,
		"title":        res.Title,
		"clean_text":   res.CleanText,
		"snippet":      res.Snippet,
		"content_type": res.ContentType,
		"retrieved_at": res.RetrievedAt.Format(time.RFC3339),
		"size_bytes":   res.SizeBytes,
		"truncated":    res.Truncated,
		"citation":     citation,
		"citations":    []map[string]any{citation},
	}, nil
}
