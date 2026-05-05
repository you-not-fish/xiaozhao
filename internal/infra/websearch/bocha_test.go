package websearch

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

func TestBochaProviderSearchNormalizesResponse(t *testing.T) {
	var gotAuth string
	var gotReq bochaRequest
	p := NewBochaProvider(BochaConfig{BaseURL: "https://bocha.test/v1/web-search", APIKey: "secret", Timeout: time.Second, MaxResults: 5})
	p.hc.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		if err := json.NewDecoder(req.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		body := []byte(`{"queryContext":{"originalQuery":"阿里 ESG"},"webPages":{"value":[{"name":"阿里巴巴 ESG 报告","url":"https://www.alibabagroup.com/esg","siteName":"阿里巴巴集团","siteIcon":"https://example.com/favicon.ico","snippet":"报告重点","summary":"减碳与普惠","datePublished":"2024-07-22T00:00:00+08:00","score":0.92}]}}`)
		return jsonResponse(http.StatusOK, body), nil
	})

	res, err := p.Search(t.Context(), SearchRequest{Query: "阿里 ESG", Count: 3, Freshness: "oneYear"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotReq.Query != "阿里 ESG" || gotReq.Count != 3 || gotReq.Freshness != "oneYear" || !gotReq.Summary {
		t.Fatalf("request = %#v", gotReq)
	}
	if res.Provider != "bocha" || len(res.Results) != 1 || res.Results[0].Title != "阿里巴巴 ESG 报告" {
		t.Fatalf("response = %#v", res)
	}
}

func TestBochaProviderMapsErrors(t *testing.T) {
	cases := []struct {
		status int
		code   errcode.Code
	}{
		{http.StatusUnauthorized, errcode.CodeToolForbidden},
		{http.StatusTooManyRequests, errcode.CodeRateLimited},
		{http.StatusInternalServerError, errcode.CodeUnavailable},
	}
	for _, tc := range cases {
		p := NewBochaProvider(BochaConfig{BaseURL: "https://bocha.test/v1/web-search", APIKey: "secret", Timeout: time.Second})
		p.hc.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(tc.status, []byte(`{"error":"bad"}`)), nil
		})
		_, err := p.Search(t.Context(), SearchRequest{Query: "test"})
		if errcode.CodeOf(err) != tc.code {
			t.Fatalf("status %d code = %s, want %s", tc.status, errcode.CodeOf(err), tc.code)
		}
	}
}

func jsonResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
