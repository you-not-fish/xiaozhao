package websearch

import (
	"context"
	"strings"
	"time"
)

type Provider interface {
	Name() string
	Search(ctx context.Context, in SearchRequest) (*SearchResponse, error)
}

type SearchRequest struct {
	Query          string
	Count          int
	Freshness      string
	IncludeDomains []string
	ExcludeDomains []string
}

type SearchResponse struct {
	Query       string
	Provider    string
	Results     []Result
	RetrievedAt time.Time
}

type Result struct {
	Title       string
	URL         string
	Snippet     string
	Summary     string
	SiteName    string
	SiteIcon    string
	PublishedAt string
	Score       float64
}

func NormalizeRequest(in SearchRequest, maxResults int, defaultFreshness string) SearchRequest {
	in.Query = strings.TrimSpace(in.Query)
	if in.Count <= 0 {
		in.Count = 5
	}
	if maxResults <= 0 || maxResults > 10 {
		maxResults = 10
	}
	if in.Count > maxResults {
		in.Count = maxResults
	}
	if in.Freshness == "" {
		in.Freshness = defaultFreshness
	}
	if in.Freshness == "" {
		in.Freshness = "noLimit"
	}
	in.IncludeDomains = cleanDomains(in.IncludeDomains)
	in.ExcludeDomains = cleanDomains(in.ExcludeDomains)
	return in
}

func cleanDomains(domains []string) []string {
	out := make([]string, 0, len(domains))
	seen := map[string]bool{}
	for _, d := range domains {
		d = strings.TrimSpace(strings.ToLower(d))
		d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		d = strings.Trim(d, "/")
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}
