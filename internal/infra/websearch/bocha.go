package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type BochaConfig struct {
	BaseURL          string
	APIKey           string
	Timeout          time.Duration
	MaxResults       int
	DefaultFreshness string
}

type BochaProvider struct {
	cfg BochaConfig
	hc  *http.Client
}

func NewBochaProvider(cfg BochaConfig) *BochaProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.bochaai.com/v1/web-search"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &BochaProvider{cfg: cfg, hc: &http.Client{Timeout: cfg.Timeout}}
}

func (p *BochaProvider) Name() string { return "bocha" }

func (p *BochaProvider) Search(ctx context.Context, in SearchRequest) (*SearchResponse, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return nil, errcode.New(errcode.CodeToolForbidden, "bocha api key is required")
	}
	in = NormalizeRequest(in, p.cfg.MaxResults, p.cfg.DefaultFreshness)
	if in.Query == "" {
		return nil, errcode.New(errcode.CodeToolParamInvalid, "query is required")
	}
	// Bocha Web Search API 当前没有 include/exclude domains 参数，
	// 域名白名单/黑名单只能在返回结果上过滤（见 normalizeBochaResults → domainAllowed）。
	// 当 Bocha 放开参数后再迁移到请求侧过滤，以节省配额和带宽。
	reqBody := bochaRequest{
		Query:     in.Query,
		Freshness: in.Freshness,
		Summary:   true,
		Count:     in.Count,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeToolParamInvalid, "marshal bocha request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeToolParamInvalid, "create bocha request")
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errcode.Wrap(err, errcode.CodeToolTimeout, "bocha search timeout")
		}
		return nil, errcode.Wrap(err, errcode.CodeUnavailable, "bocha search unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapBochaHTTPError(resp)
	}
	limited := io.LimitReader(resp.Body, 4*1024*1024)
	var out bochaResponse
	if err := json.NewDecoder(limited).Decode(&out); err != nil {
		return nil, errcode.Wrap(err, errcode.CodeToolParamInvalid, "decode bocha response")
	}
	results := normalizeBochaResults(out.WebPages.Value, in)
	return &SearchResponse{
		Query:       firstNonEmpty(out.QueryContext.OriginalQuery, in.Query),
		Provider:    p.Name(),
		Results:     results,
		RetrievedAt: time.Now(),
	}, nil
}

type bochaRequest struct {
	Query     string `json:"query"`
	Freshness string `json:"freshness,omitempty"`
	Summary   bool   `json:"summary"`
	Count     int    `json:"count"`
}

type bochaResponse struct {
	QueryContext struct {
		OriginalQuery string `json:"originalQuery"`
	} `json:"queryContext"`
	WebPages struct {
		Value []bochaResult `json:"value"`
	} `json:"webPages"`
}

type bochaResult struct {
	Name          string  `json:"name"`
	URL           string  `json:"url"`
	SiteName      string  `json:"siteName"`
	SiteIcon      string  `json:"siteIcon"`
	Snippet       string  `json:"snippet"`
	Summary       string  `json:"summary"`
	DatePublished string  `json:"datePublished"`
	Score         float64 `json:"score"`
}

func normalizeBochaResults(items []bochaResult, in SearchRequest) []Result {
	out := make([]Result, 0, len(items))
	for _, item := range items {
		if item.URL == "" || !domainAllowed(item.URL, in.IncludeDomains, in.ExcludeDomains) {
			continue
		}
		out = append(out, Result{
			Title:       strings.TrimSpace(item.Name),
			URL:         strings.TrimSpace(item.URL),
			Snippet:     strings.TrimSpace(item.Snippet),
			Summary:     strings.TrimSpace(item.Summary),
			SiteName:    strings.TrimSpace(item.SiteName),
			SiteIcon:    strings.TrimSpace(item.SiteIcon),
			PublishedAt: strings.TrimSpace(item.DatePublished),
			Score:       item.Score,
		})
	}
	return out
}

func mapBochaHTTPError(resp *http.Response) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := fmt.Sprintf("bocha search failed with status %d", resp.StatusCode)
	if len(snippet) > 0 {
		msg += ": " + string(snippet)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errcode.New(errcode.CodeToolForbidden, msg)
	case http.StatusTooManyRequests:
		return errcode.New(errcode.CodeRateLimited, msg)
	case http.StatusBadRequest:
		return errcode.New(errcode.CodeToolParamInvalid, msg)
	default:
		return errcode.New(errcode.CodeUnavailable, msg)
	}
}

func domainAllowed(rawURL string, includeDomains, excludeDomains []string) bool {
	host := hostFromURL(rawURL)
	if host == "" {
		return false
	}
	for _, d := range excludeDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return false
		}
	}
	if len(includeDomains) == 0 {
		return true
	}
	for _, d := range includeDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func hostFromURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
