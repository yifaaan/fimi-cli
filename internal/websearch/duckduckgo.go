package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"fimi-cli/internal/tools"
)

const defaultDuckDuckGoPath = "/html/"

// DuckDuckGoConfig 描述 DuckDuckGo 搜索适配器需要的最小 HTTP 配置。
type DuckDuckGoConfig struct {
	BaseURL   string
	UserAgent string
	Client    *http.Client
}

// DuckDuckGoSearcher 是一个可替换的外部搜索适配器。
// 它属于基础设施层，实现 tools.WebSearcher 这个应用边界。
type DuckDuckGoSearcher struct {
	baseURL   string
	userAgent string
	client    *http.Client
}

type duckDuckGoResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Content string `json:"content"`
}

type duckDuckGoResponse struct {
	Results []duckDuckGoResult `json:"results"`
}

func NewDuckDuckGoSearcher(cfg DuckDuckGoConfig) (*DuckDuckGoSearcher, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("duckduckgo base url is required")
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		return nil, fmt.Errorf("duckduckgo user agent is required")
	}
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse duckduckgo base url: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("duckduckgo base url must include scheme and host")
	}

	return &DuckDuckGoSearcher{
		baseURL:   parsedURL.String(),
		userAgent: userAgent,
		client:    client,
	}, nil
}

func (s *DuckDuckGoSearcher) Search(
	ctx context.Context,
	query string,
	limit int,
	includeContent bool,
) ([]tools.WebSearchResult, error) {
	form := url.Values{}
	form.Set("q", query)
	form.Set("limit", fmt.Sprintf("%d", limit))
	if includeContent {
		form.Set("include_content", "true")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build duckduckgo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request duckduckgo search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return nil, fmt.Errorf("duckduckgo search failed with status %d", resp.StatusCode)
		}
		message := strings.TrimSpace(string(body))
		if message == "" {
			return nil, fmt.Errorf("duckduckgo search failed with status %d", resp.StatusCode)
		}

		return nil, fmt.Errorf("duckduckgo search failed with status %d: %s", resp.StatusCode, message)
	}

	var payload duckDuckGoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode duckduckgo response: %w", err)
	}

	results := make([]tools.WebSearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		results = append(results, tools.WebSearchResult{
			Title:   strings.TrimSpace(item.Title),
			URL:     strings.TrimSpace(item.URL),
			Snippet: strings.TrimSpace(item.Snippet),
			Content: strings.TrimSpace(item.Content),
		})
	}

	return results, nil
}
