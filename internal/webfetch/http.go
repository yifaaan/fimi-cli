package webfetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fimi-cli/internal/tools"
	"golang.org/x/net/html"
)

const (
	defaultFetchTimeout = 30 * time.Second
	defaultMaxFetchSize = 10 * 1024 * 1024 // 10MB
	defaultUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
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

	// 限制读取大小，避免把超大页面完整读入内存。
	limitedReader := io.LimitReader(resp.Body, f.maxSize)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	if len(content) == 0 {
		return formatFetchedContent(rawURL, "", "Fetched page is empty."), nil
	}

	if extracted, title, applied, err := extractReadableText(content, resp.Header.Get("Content-Type")); err != nil {
		return "", err
	} else if applied {
		return formatFetchedContent(rawURL, title, extracted), nil
	}

	return formatFetchedContent(rawURL, "", string(content)), nil
}

func extractReadableText(body []byte, contentType string) (string, string, bool, error) {
	if !isHTMLContentType(contentType, body) {
		return "", "", false, nil
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", "", false, fmt.Errorf("parse html document: %w", err)
	}

	title := extractDocumentTitle(doc)
	contentRoot := selectContentRoot(doc)
	if contentRoot == nil {
		return "No readable main text could be extracted from the page.", title, true, nil
	}

	text := normalizeReadableWhitespace(renderNodeText(contentRoot))
	if text == "" {
		return "No readable main text could be extracted from the page.", title, true, nil
	}

	return text, title, true, nil
}

func isHTMLContentType(contentType string, body []byte) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml") {
		return true
	}
	if contentType != "" {
		return false
	}

	prefix := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 256)])))
	return strings.HasPrefix(prefix, "<!doctype html") || strings.HasPrefix(prefix, "<html")
}

func selectContentRoot(doc *html.Node) *html.Node {
	if mainNode := findFirstElement(doc, "main"); mainNode != nil {
		return mainNode
	}
	if articleNode := findFirstElement(doc, "article"); articleNode != nil {
		return articleNode
	}

	bodyNode := findFirstElement(doc, "body")
	if bodyNode == nil {
		return nil
	}

	bestNode := bodyNode
	bestScore := scoreNode(bestNode)
	for _, candidate := range findCandidateContentNodes(bodyNode) {
		score := scoreNode(candidate)
		if score > bestScore {
			bestNode = candidate
			bestScore = score
		}
	}

	return bestNode
}

func findCandidateContentNodes(root *html.Node) []*html.Node {
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "section", "div", "article", "main":
				nodes = append(nodes, node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	return nodes
}

func scoreNode(node *html.Node) int {
	if node == nil {
		return 0
	}

	textLen := visibleTextLength(node)
	if textLen == 0 {
		return 0
	}

	linkLen := linkTextLength(node)
	paragraphCount := descendantElementCount(node, "p")
	headingCount := descendantHeadingCount(node)

	return textLen + paragraphCount*120 + headingCount*80 - linkLen*2
}

func visibleTextLength(node *html.Node) int {
	if node == nil {
		return 0
	}
	if node.Type == html.TextNode {
		return len(strings.TrimSpace(node.Data))
	}
	if shouldSkipNode(node) {
		return 0
	}

	total := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		total += visibleTextLength(child)
	}

	return total
}

func linkTextLength(node *html.Node) int {
	if node == nil {
		return 0
	}
	if node.Type == html.ElementNode && node.Data == "a" {
		return visibleTextLength(node)
	}

	total := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		total += linkTextLength(child)
	}

	return total
}

func descendantElementCount(node *html.Node, tag string) int {
	if node == nil {
		return 0
	}

	count := 0
	if node.Type == html.ElementNode && node.Data == tag {
		count++
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		count += descendantElementCount(child, tag)
	}

	return count
}

func descendantHeadingCount(node *html.Node) int {
	if node == nil {
		return 0
	}

	count := 0
	if node.Type == html.ElementNode && isHeadingTag(node.Data) {
		count++
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		count += descendantHeadingCount(child)
	}

	return count
}

func isHeadingTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	default:
		return false
	}
}

func renderNodeText(root *html.Node) string {
	var builder strings.Builder
	writeNodeText(&builder, root)

	return builder.String()
}

func writeNodeText(builder *strings.Builder, node *html.Node) {
	if node == nil {
		return
	}
	if shouldSkipNode(node) {
		return
	}
	if node.Type == html.TextNode {
		text := strings.TrimSpace(node.Data)
		if text != "" {
			if builder.Len() > 0 && !endsWithWhitespace(builder.String()) {
				builder.WriteByte(' ')
			}
			builder.WriteString(text)
		}
		return
	}
	if node.Type != html.ElementNode {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			writeNodeText(builder, child)
		}
		return
	}

	if isBlockElement(node.Data) && builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n\n") {
		builder.WriteString("\n\n")
	}
	if node.Data == "br" && !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteByte('\n')
	}
	if node.Data == "li" {
		builder.WriteString("- ")
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeNodeText(builder, child)
	}

	if isBlockElement(node.Data) && builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n\n") {
		builder.WriteString("\n\n")
	}
}

func shouldSkipNode(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	switch node.Data {
	case "script", "style", "noscript", "svg", "canvas", "iframe", "nav", "footer", "header", "form", "aside":
		return true
	default:
		return false
	}
}

func isBlockElement(tag string) bool {
	switch tag {
	case "main", "article", "section", "div", "p", "ul", "ol", "li", "table", "tr", "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	default:
		return false
	}
}

func findFirstElement(root *html.Node, tag string) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && root.Data == tag {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstElement(child, tag); found != nil {
			return found
		}
	}

	return nil
}

func extractDocumentTitle(doc *html.Node) string {
	titleNode := findFirstElement(doc, "title")
	if titleNode == nil {
		return ""
	}

	return normalizeReadableWhitespace(renderNodeText(titleNode))
}

func formatFetchedContent(rawURL, title, content string) string {
	var builder strings.Builder
	if title != "" {
		builder.WriteString("Title: ")
		builder.WriteString(title)
		builder.WriteString("\n")
	}
	builder.WriteString("URL: ")
	builder.WriteString(strings.TrimSpace(rawURL))

	content = strings.TrimSpace(content)
	if content != "" {
		builder.WriteString("\n\n")
		builder.WriteString(content)
	}

	return builder.String()
}

func normalizeReadableWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	normalized := make([]string, 0, len(lines))
	blankRun := 0
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			blankRun++
			if blankRun <= 1 {
				normalized = append(normalized, "")
			}
			continue
		}
		blankRun = 0
		normalized = append(normalized, line)
	}

	return strings.TrimSpace(strings.Join(normalized, "\n"))
}

func endsWithWhitespace(text string) bool {
	if text == "" {
		return false
	}
	last := text[len(text)-1]
	return last == ' ' || last == '\n' || last == '\t'
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 确保实现接口
var _ tools.URLFetcher = (*HTTPFetcher)(nil)
