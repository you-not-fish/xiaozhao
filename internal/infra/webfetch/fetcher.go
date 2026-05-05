package webfetch

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"

	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

type Config struct {
	Timeout       time.Duration
	MaxBodyBytes  int64
	MaxRedirects  int
	AllowedPorts  map[string]bool
	Transport     http.RoundTripper
	ResolveIPFunc func(ctx context.Context, host string) ([]net.IP, error)
}

type Fetcher struct {
	cfg Config
	hc  *http.Client
}

type Request struct {
	URL      string
	Query    string
	MaxChars int
}

type Response struct {
	URL          string
	FinalURL     string
	Title        string
	CleanText    string
	Snippet      string
	ContentType  string
	RetrievedAt  time.Time
	SizeBytes    int64
	Truncated    bool
	ProviderName string
}

func New(cfg Config) *Fetcher {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1024 * 1024
	}
	if cfg.MaxRedirects <= 0 {
		cfg.MaxRedirects = 3
	}
	if cfg.AllowedPorts == nil {
		cfg.AllowedPorts = map[string]bool{"": true, "80": true, "443": true}
	}
	resolver := cfg.ResolveIPFunc
	if resolver == nil {
		resolver = func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	cfg.ResolveIPFunc = resolver
	f := &Fetcher{cfg: cfg}
	f.hc = &http.Client{
		Timeout: cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= cfg.MaxRedirects {
				return errors.New("too many redirects")
			}
			// 每次跳转后都重新校验目标 URL，避免先访问公网再被重定向到内网地址。
			if err := f.ValidateURL(req.Context(), req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
	if cfg.Transport != nil {
		f.hc.Transport = cfg.Transport
	}
	return f
}

func (f *Fetcher) Fetch(ctx context.Context, in Request) (*Response, error) {
	if err := f.ValidateURL(ctx, in.URL); err != nil {
		return nil, err
	}
	if in.MaxChars <= 0 {
		in.MaxChars = 4000
	}
	if in.MaxChars < 500 {
		in.MaxChars = 500
	}
	if in.MaxChars > 8000 {
		in.MaxChars = 8000
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeToolParamInvalid, "create fetch request")
	}
	req.Header.Set("User-Agent", "xiaozhao-agent-web-fetch/1.0")
	req.Header.Set("Accept", "text/html,text/plain,application/json;q=0.9,*/*;q=0.1")
	resp, err := f.hc.Do(req)
	if err != nil {
		if e, ok := errcode.As(err); ok {
			return nil, e
		}
		if ctx.Err() != nil {
			return nil, errcode.Wrap(err, errcode.CodeToolTimeout, "web fetch timeout")
		}
		return nil, errcode.Wrap(err, errcode.CodeUnavailable, "web fetch failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errcode.Newf(errcode.CodeUnavailable, "web fetch status %d", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !allowedContentType(ct) {
		return nil, errcode.Newf(errcode.CodeToolParamInvalid, "unsupported content type %q", ct)
	}
	limited := &limitReader{r: resp.Body, n: f.cfg.MaxBodyBytes + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, errcode.Wrap(err, errcode.CodeUnavailable, "read web response")
	}
	truncatedBody := int64(len(body)) > f.cfg.MaxBodyBytes
	if truncatedBody {
		body = body[:f.cfg.MaxBodyBytes]
	}
	title, text := extractText(ct, string(body))
	text = normalizeSpace(text)
	truncatedText := false
	if len([]rune(text)) > in.MaxChars {
		text = string([]rune(text)[:in.MaxChars])
		truncatedText = true
	}
	return &Response{
		URL:         in.URL,
		FinalURL:    resp.Request.URL.String(),
		Title:       title,
		CleanText:   text,
		Snippet:     makeSnippet(text, in.Query, 500),
		ContentType: ct,
		RetrievedAt: time.Now(),
		SizeBytes:   int64(len(body)),
		Truncated:   truncatedBody || truncatedText,
	}, nil
}

func (f *Fetcher) ValidateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errcode.New(errcode.CodeToolParamInvalid, "url is invalid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errcode.New(errcode.CodeToolParamInvalid, "only http and https are allowed")
	}
	port := u.Port()
	if !f.cfg.AllowedPorts[port] {
		return errcode.New(errcode.CodeToolForbidden, "url port is not allowed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errcode.New(errcode.CodeToolForbidden, "localhost is not allowed")
	}
	ips, err := f.cfg.ResolveIPFunc(ctx, host)
	if err != nil || len(ips) == 0 {
		return errcode.Wrap(err, errcode.CodeUnavailable, "resolve url host")
	}
	for _, ip := range ips {
		// web_fetch 访问公网前必须阻断内网、环回、链路本地和云厂商 metadata 地址，避免 SSRF。
		if isPrivateIP(ip) {
			return errcode.New(errcode.CodeToolForbidden, "private address is not allowed")
		}
	}
	return nil
}

type limitReader struct {
	r io.Reader
	n int64
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

func allowedContentType(ct string) bool {
	// 空 Content-Type 一律拒绝：无法确认是否为文本，二进制响应容易被当乱码回传给模型。
	// 如果确有正当站点不带 CT，可以在调用方显式 sniff 后再决定。
	if ct == "" {
		return false
	}
	return strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "text/plain") ||
		strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "application/xhtml") ||
		strings.HasPrefix(ct, "text/")
}

func extractText(contentType, body string) (string, string) {
	if !strings.Contains(contentType, "text/html") {
		return "", body
	}
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "", body
	}
	var title strings.Builder
	var text strings.Builder
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			name := strings.ToLower(n.Data)
			if name == "script" || name == "style" || name == "noscript" || name == "svg" || name == "nav" || name == "footer" {
				skip = true
			}
			if name == "title" {
				appendNodeText(n, &title)
				return
			}
		}
		if !skip && n.Type == html.TextNode {
			text.WriteString(n.Data)
			text.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
	}
	walk(doc, false)
	return normalizeSpace(title.String()), text.String()
}

func appendNodeText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		b.WriteByte(' ')
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		appendNodeText(c, b)
	}
}

func normalizeSpace(s string) string {
	return strings.TrimSpace(strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " "))
}

func makeSnippet(text, query string, max int) string {
	rs := []rune(text)
	if len(rs) <= max {
		return text
	}
	q := strings.TrimSpace(query)
	if q != "" {
		idx := strings.Index(strings.ToLower(text), strings.ToLower(q))
		if idx >= 0 {
			prefix := len([]rune(text[:idx])) - max/4
			if prefix < 0 {
				prefix = 0
			}
			end := prefix + max
			if end > len(rs) {
				end = len(rs)
			}
			return string(rs[prefix:end])
		}
	}
	return string(rs[:max])
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		for _, block := range privateIPv4Blocks {
			if block.Contains(ip4) {
				return true
			}
		}
		return false
	}
	for _, block := range privateIPv6Blocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

var (
	privateIPv4Blocks []*net.IPNet
	privateIPv6Blocks []*net.IPNet
)

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"100.64.0.0/10",
		"0.0.0.0/8",
	} {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			privateIPv4Blocks = append(privateIPv4Blocks, block)
		}
	}
	for _, cidr := range []string{"::1/128", "fc00::/7", "fe80::/10", "::/128"} {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			privateIPv6Blocks = append(privateIPv6Blocks, block)
		}
	}
}
