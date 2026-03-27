package websearch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"fimi-cli/internal/tools"
	"golang.org/x/net/html"
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
	if strings.TrimSpace(parsedURL.Path) == "" {
		parsedURL.Path = defaultDuckDuckGoPath
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
	_ = includeContent

	requestURL, err := buildDuckDuckGoSearchURL(s.baseURL, query)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build duckduckgo request: %w", err)
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read duckduckgo response: %w", err)
	}

	results, err := parseDuckDuckGoHTML(body, limit)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func buildDuckDuckGoSearchURL(baseURL, query string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse duckduckgo base url: %w", err)
	}

	values := parsedURL.Query()
	values.Set("q", query)
	parsedURL.RawQuery = values.Encode()

	return parsedURL.String(), nil
}

func parseDuckDuckGoHTML(body []byte, limit int) ([]tools.WebSearchResult, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse duckduckgo response html: %w", err)
	}

	resultNodes := findAllNodes(doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && hasClass(node, "result") && hasClass(node, "web-result")
	})

	results := make([]tools.WebSearchResult, 0, len(resultNodes))
	for _, resultNode := range resultNodes {
		titleNode := findFirstNode(resultNode, func(node *html.Node) bool {
			return node.Type == html.ElementNode && node.Data == "a" && hasClass(node, "result__a")
		})
		if titleNode == nil {
			continue
		}

		snippetNode := findFirstNode(resultNode, func(node *html.Node) bool {
			return node.Type == html.ElementNode && node.Data == "a" && hasClass(node, "result__snippet")
		})

		result := tools.WebSearchResult{
			Title:   strings.TrimSpace(nodeText(titleNode)),
			URL:     extractDuckDuckGoResultURL(attributeValue(titleNode, "href")),
			Snippet: strings.TrimSpace(nodeText(snippetNode)),
		}
		if result.Title == "" && result.URL == "" && result.Snippet == "" {
			continue
		}

		results = append(results, result)
		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results, nil
}

func extractDuckDuckGoResultURL(rawHref string) string {
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return ""
	}
	if strings.HasPrefix(rawHref, "//") {
		rawHref = "https:" + rawHref
	}

	parsedURL, err := url.Parse(rawHref)
	if err == nil {
		target := strings.TrimSpace(parsedURL.Query().Get("uddg"))
		if target != "" {
			return target
		}
		if parsedURL.Scheme != "" && parsedURL.Host != "" {
			return parsedURL.String()
		}
	}

	return rawHref
}

func findAllNodes(root *html.Node, match func(*html.Node) bool) []*html.Node {
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if match(node) {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	return nodes
}

func findFirstNode(root *html.Node, match func(*html.Node) bool) *html.Node {
	if root == nil {
		return nil
	}
	if match(root) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstNode(child, match); found != nil {
			return found
		}
	}

	return nil
}

func hasClass(node *html.Node, className string) bool {
	for _, attr := range node.Attr {
		if attr.Key != "class" {
			continue
		}
		for _, item := range strings.Fields(attr.Val) {
			if item == className {
				return true
			}
		}
	}

	return false
}

func attributeValue(node *html.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}

	return ""
}

func nodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}

	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(nodeText(child))
	}

	return builder.String()
}
