package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"fimi-cli/internal/runtime"
)

func appendShapeMessage(output, message string) string {
	if message == "" {
		return output
	}

	return output + "\n\n[" + message + "]"
}

func newReadFileHandler(workDir string, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeReadFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		path, err := resolveWorkspacePath(workDir, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return runtime.ToolExecution{
				Call:   call,
				Output: fmt.Sprintf("Error reading file %q: %v", path, err),
			}, nil
		}

		shaped := shaper.Shape(string(data))
		return runtime.ToolExecution{
			Call:          call,
			Output:        appendShapeMessage(shaped.Output, shaped.Message),
			DisplayOutput: buildInlinePreview("", string(data)),
		}, nil
	}
}

func newGlobHandler(workDir string, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeGlobArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		pattern, err := normalizeWorkspacePattern(args.Pattern)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		matches, err := findGlobMatches(rootAbs, pattern)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		shaped := shaper.Shape(strings.Join(matches, "\n"))
		return runtime.ToolExecution{
			Call:          call,
			Output:        appendShapeMessage(shaped.Output, shaped.Message),
			DisplayOutput: buildInlinePreview("", strings.Join(matches, "\n")),
		}, nil
	}
}

func newGrepHandler(workDir string, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeGrepArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		expression, err := regexp.Compile(args.Pattern)
		if err != nil {
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: compile grep pattern: %v", ErrToolArgumentsInvalid, err))
		}

		matches, err := findGrepMatches(rootAbs, targetAbs, expression)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		shaped := shaper.Shape(strings.Join(matches, "\n"))
		return runtime.ToolExecution{
			Call:          call,
			Output:        appendShapeMessage(shaped.Output, shaped.Message),
			DisplayOutput: buildInlinePreview("", strings.Join(matches, "\n")),
		}, nil
	}
}

func newSearchWebHandler(searcher WebSearcher, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeSearchWebArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		if searcher == nil {
			return runtime.ToolExecution{}, markTemporary(errors.New("web search backend is not configured"))
		}

		results, err := searcher.Search(ctx, args.Query, args.Limit, args.IncludeContent)
		if err != nil {
			return runtime.ToolExecution{}, markTemporary(fmt.Errorf("search web: %w", err))
		}

		shaped := shaper.Shape(formatWebSearchResults(results, args.IncludeContent))
		return runtime.ToolExecution{
			Call:          call,
			Output:        appendShapeMessage(shaped.Output, shaped.Message),
			DisplayOutput: buildInlinePreview("", formatWebSearchPreview(results, args.IncludeContent)),
		}, nil
	}
}

func NewSearchWebHandler(searcher WebSearcher, shaper OutputShaper) HandlerFunc {
	return newSearchWebHandler(searcher, shaper)
}

func newFetchURLHandler(fetcher URLFetcher, shaper OutputShaper) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeFetchURLArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		if fetcher == nil {
			return runtime.ToolExecution{}, markTemporary(errors.New("url fetcher is not configured"))
		}

		content, err := fetcher.Fetch(ctx, args.URL)
		if err != nil {
			return runtime.ToolExecution{}, markTemporary(fmt.Errorf("fetch url: %w", err))
		}

		shaped := shaper.Shape(content)
		return runtime.ToolExecution{
			Call:          call,
			Output:        appendShapeMessage(shaped.Output, shaped.Message),
			DisplayOutput: buildInlinePreview("", content),
		}, nil
	}
}

func NewFetchURLHandler(fetcher URLFetcher, shaper OutputShaper) HandlerFunc {
	return newFetchURLHandler(fetcher, shaper)
}

func formatWebSearchResults(results []WebSearchResult, includeContent bool) string {
	if len(results) == 0 {
		return "No web results found."
	}

	var builder strings.Builder
	for i, result := range results {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(strconv.Itoa(i + 1))
		builder.WriteString(". ")

		title := strings.TrimSpace(result.Title)
		if title == "" {
			title = strings.TrimSpace(result.URL)
		}
		if title == "" {
			title = "Untitled result"
		}
		builder.WriteString(title)

		url := strings.TrimSpace(result.URL)
		if url != "" {
			builder.WriteString("\nURL: ")
			builder.WriteString(url)
		}

		snippet := strings.TrimSpace(result.Snippet)
		if snippet != "" {
			builder.WriteString("\nSnippet: ")
			builder.WriteString(snippet)
		}

		if includeContent {
			content := strings.TrimSpace(result.Content)
			if content != "" {
				builder.WriteString("\nContent: ")
				builder.WriteString(content)
			}
		}
	}

	return builder.String()
}

func formatWebSearchPreview(results []WebSearchResult, includeContent bool) string {
	if len(results) == 0 {
		return "No web results found."
	}

	var lines []string
	for i, result := range results {
		title := strings.TrimSpace(result.Title)
		if title == "" {
			title = strings.TrimSpace(result.URL)
		}
		if title == "" {
			title = "Untitled result"
		}
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, title))
		if snippet := strings.TrimSpace(result.Snippet); snippet != "" {
			lines = append(lines, snippet)
		}
		if includeContent {
			if content := strings.TrimSpace(result.Content); content != "" {
				lines = append(lines, content)
			}
		}
	}

	return strings.Join(lines, "\n")
}

func decodeSearchWebArguments(raw string) (searchWebArguments, error) {
	var args searchWebArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return searchWebArguments{}, markRefused(fmt.Errorf("%w: decode search_web arguments: %v", ErrToolArgumentsInvalid, err))
	}

	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return searchWebArguments{}, markRefused(ErrToolSearchQueryRequired)
	}
	if args.Limit == 0 {
		args.Limit = 5
	}
	if args.Limit < 1 || args.Limit > 20 {
		return searchWebArguments{}, markRefused(fmt.Errorf("%w: %d", ErrToolSearchLimitInvalid, args.Limit))
	}

	return args, nil
}

func decodeGlobArguments(raw string) (globArguments, error) {
	var args globArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return globArguments{}, markRefused(fmt.Errorf("%w: decode glob arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return globArguments{}, markRefused(ErrToolPatternRequired)
	}

	return args, nil
}

func decodeGrepArguments(raw string) (grepArguments, error) {
	var args grepArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return grepArguments{}, markRefused(fmt.Errorf("%w: decode grep arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return grepArguments{}, markRefused(ErrToolPatternRequired)
	}
	if strings.TrimSpace(args.Path) == "" {
		args.Path = "."
	}

	return args, nil
}

func decodeReadFileArguments(raw string) (readFileArguments, error) {
	var args readFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return readFileArguments{}, markRefused(fmt.Errorf("%w: decode read_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return readFileArguments{}, markRefused(ErrToolPathRequired)
	}

	return args, nil
}

func decodeFetchURLArguments(raw string) (fetchURLArguments, error) {
	var args fetchURLArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return fetchURLArguments{}, markRefused(fmt.Errorf("%w: decode fetch_url arguments: %v", ErrToolArgumentsInvalid, err))
	}

	args.URL = strings.TrimSpace(args.URL)
	if args.URL == "" {
		return fetchURLArguments{}, markRefused(ErrToolURLRequired)
	}

	return args, nil
}

func resolveWorkspaceRoot(workDir string) (string, error) {
	root := workDir
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	return rootAbs, nil
}

func resolveWorkspacePath(workDir string, target string) (string, error) {
	rootAbs, err := resolveWorkspaceRoot(workDir)
	if err != nil {
		return "", err
	}

	targetPath := target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(rootAbs, targetPath)
	}

	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve tool path %q: %w", target, err)
	}

	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("relativize tool path %q: %w", targetAbs, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", markRefused(fmt.Errorf("%w: %s", ErrToolPathOutsideWorkspace, target))
	}

	return targetAbs, nil
}

func relativeWorkspacePath(rootAbs string, targetAbs string) (string, error) {
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("relativize tool path %q: %w", targetAbs, err)
	}

	return filepath.ToSlash(rel), nil
}

func normalizeWorkspacePattern(raw string) (string, error) {
	pattern := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
	if pattern == "." {
		return pattern, nil
	}
	if path.IsAbs(pattern) || pattern == ".." || strings.HasPrefix(pattern, "../") {
		return "", markRefused(fmt.Errorf("%w: %s", ErrToolPatternOutsideWorkspace, raw))
	}

	return pattern, nil
}

func findGlobMatches(rootAbs string, pattern string) ([]string, error) {
	matches := make([]string, 0)
	err := filepath.WalkDir(rootAbs, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(rootAbs, current)
		if err != nil {
			return fmt.Errorf("relativize path %q: %w", current, err)
		}
		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)
		ok, err := matchWorkspacePattern(pattern, rel)
		if err != nil {
			return markRefused(fmt.Errorf("%w: match glob pattern %q: %v", ErrToolArgumentsInvalid, pattern, err))
		}
		if ok {
			matches = append(matches, rel)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace for glob: %w", err)
	}

	return matches, nil
}

func matchWorkspacePattern(pattern string, target string) (bool, error) {
	if pattern == "." {
		return target == ".", nil
	}

	patternSegments := splitSlashPath(pattern)
	targetSegments := splitSlashPath(target)

	return matchWorkspacePatternSegments(patternSegments, targetSegments)
}

func matchWorkspacePatternSegments(pattern []string, target []string) (bool, error) {
	if len(pattern) == 0 {
		return len(target) == 0, nil
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(target); i++ {
			ok, err := matchWorkspacePatternSegments(pattern[1:], target[i:])
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}

		return false, nil
	}
	if len(target) == 0 {
		return false, nil
	}

	ok, err := path.Match(pattern[0], target[0])
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	return matchWorkspacePatternSegments(pattern[1:], target[1:])
}

func splitSlashPath(raw string) []string {
	if raw == "" || raw == "." {
		return nil
	}

	return strings.Split(raw, "/")
}

func findGrepMatches(rootAbs string, targetAbs string, expression *regexp.Regexp) ([]string, error) {
	info, err := os.Stat(targetAbs)
	if err != nil {
		return nil, fmt.Errorf("stat grep path %q: %w", targetAbs, err)
	}

	if !info.IsDir() {
		return grepFile(rootAbs, targetAbs, expression)
	}

	matches := make([]string, 0)
	err = filepath.WalkDir(targetAbs, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		fileMatches, err := grepFile(rootAbs, current, expression)
		if err != nil {
			return err
		}
		matches = append(matches, fileMatches...)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace for grep: %w", err)
	}

	return matches, nil
}

func grepFile(rootAbs string, filePath string, expression *regexp.Regexp) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open grep file %q: %w", filePath, err)
	}
	defer file.Close()

	rel, err := filepath.Rel(rootAbs, filePath)
	if err != nil {
		return nil, fmt.Errorf("relativize grep file %q: %w", filePath, err)
	}
	rel = filepath.ToSlash(rel)

	matches := make([]string, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !expression.MatchString(line) {
			continue
		}

		matches = append(matches, rel+":"+strconv.Itoa(lineNumber)+":"+line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan grep file %q: %w", filePath, err)
	}

	return matches, nil
}
