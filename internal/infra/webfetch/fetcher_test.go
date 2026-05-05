package webfetch

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

func TestValidateURLRejectsPrivateTargets(t *testing.T) {
	f := New(Config{ResolveIPFunc: func(_ context.Context, host string) ([]net.IP, error) {
		if ip := net.ParseIP(host); ip != nil {
			return []net.IP{ip}, nil
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}})
	cases := []string{
		"file:///etc/passwd",
		"http://localhost/a",
		"http://127.0.0.1/a",
		"http://10.0.0.1/a",
		"http://172.16.0.1/a",
		"http://192.168.1.1/a",
		"http://169.254.169.254/latest/meta-data",
	}
	for _, raw := range cases {
		if err := f.ValidateURL(t.Context(), raw); err == nil {
			t.Fatalf("ValidateURL(%q) succeeded, want error", raw)
		}
	}
}

func TestFetchExtractsHTMLAndRejectsRedirectToPrivateIP(t *testing.T) {
	html := `<html><head><title>测试标题</title><script>bad()</script></head><body><nav>导航</nav><main>这是正文，包含退款政策。</main><footer>页脚</footer></body></html>`
	f := New(Config{
		Timeout:      time.Second,
		MaxBodyBytes: 1024,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "example.com" && req.URL.Path == "/redirect" {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"http://169.254.169.254/latest"}},
					Body:       io.NopCloser(bytes.NewReader(nil)),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(html))),
				Request:    req,
			}, nil
		}),
		ResolveIPFunc: func(_ context.Context, host string) ([]net.IP, error) {
			if ip := net.ParseIP(host); ip != nil {
				return []net.IP{ip}, nil
			}
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
	})
	res, err := f.Fetch(t.Context(), Request{URL: "https://example.com/page", Query: "退款", MaxChars: 1000})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res.Title != "测试标题" || !strings.Contains(res.CleanText, "退款政策") || strings.Contains(res.CleanText, "bad()") {
		t.Fatalf("extracted response = %#v", res)
	}
	_, err = f.Fetch(t.Context(), Request{URL: "https://example.com/redirect"})
	if errcode.CodeOf(err) != errcode.CodeToolForbidden {
		t.Fatalf("redirect error code = %s, want %s", errcode.CodeOf(err), errcode.CodeToolForbidden)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
