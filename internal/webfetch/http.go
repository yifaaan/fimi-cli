package webfetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fimi-cli/internal/tools"
)

const (
	defaultFetchTimeout  = 30 * time.Second
	defaultMaxFetchSize  = 10 * 1024 * 1024 // 10MB
	defaultUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
)

// HTTPFetcherConfig 配置 HTTP URL 获取器。
type HTTPFetcherConfig struct {
	Client    *http.Client
	UserAgent string
	MaxSize   int64
}

// HTTPFetcher 使用 HTTP 获取 URL 内容。
// 它属于基础设施层，实现 tools.URLFetcher 这个应用边界。
type HTTPFetcher struct {
	client    *http.Client
	userAgent string
	maxSize   int64
}

// NewHTTPFetcher 创建一个新的 HTTP URL 获取器。
func NewHTTPFetcher(cfg HTTPFetcherConfig) (*HTTPFetcher, error) {
	client := cfg.Client
	if client == nil {
		client = &http.Client{
			Timeout: defaultFetchTimeout,
		}
	}

	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = defaultMaxFetchSize
	}

	return &HTTPFetcher{
		client:    client,
		userAgent: userAgent,
		maxSize:   maxSize,
	}, nil
}

// Fetch 实现 tools.URLFetcher 接口。
func (f *HTTPFetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("unsupported url scheme: %s (only http and https are supported)", parsedURL.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,*/*")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d: failed to fetch url (status: %s)", resp.StatusCode, resp.Status)
	}

	// 限制读取大小
	limitedReader := io.LimitReader(resp.Body, f.maxSize)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	if len(content) == 0 {
		return "The response body is empty.", nil
	}

	return string(content), nil
}

// 确保实现接口
var _ tools.URLFetcher = (*HTTPFetcher)(nil)
