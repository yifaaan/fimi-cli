package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fimi-cli/internal/runtime"
)

func TestNewBuiltinExecutorReadFileReadsWorkspaceFile(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	filePath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(filePath, []byte("hello from file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", filePath, err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReadFile,
			Kind: KindFile,
		},
	}, workDir)

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolReadFile,
		Arguments: `{"path":"notes.txt"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "hello from file\n" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "hello from file\n")
	}
}

func TestNewBuiltinExecutorReadFileRejectsPathOutsideWorkspace(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReadFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolReadFile,
		Arguments: `{"path":"../secret.txt"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestNewBuiltinExecutorBashRunsCommandInsideWorkDir(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	markerPath := filepath.Join(workDir, "marker.txt")
	if err := os.WriteFile(markerPath, []byte("marker"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", markerPath, err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolBash,
			Kind: KindCommand,
		},
	}, workDir)

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolBash,
		Arguments: `{"command":"printf '%s' \"$PWD\" && printf '\n' && ls marker.txt && printf 'warn' >&2"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(got.Stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("len(stdout lines) = %d, want %d, stdout=%q", len(lines), 2, got.Stdout)
	}
	if lines[0] != workDir {
		t.Fatalf("stdout workDir line = %q, want %q", lines[0], workDir)
	}
	if lines[1] != "marker.txt" {
		t.Fatalf("stdout file line = %q, want %q", lines[1], "marker.txt")
	}
	if got.Stderr != "warn" {
		t.Fatalf("Execute().Stderr = %q, want %q", got.Stderr, "warn")
	}
	if got.ExitCode != 0 {
		t.Fatalf("Execute().ExitCode = %d, want %d", got.ExitCode, 0)
	}
}

func TestNewBuiltinExecutorBashReturnsStructuredNonZeroExit(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolBash,
			Kind: KindCommand,
		},
	}, t.TempDir())

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolBash,
		Arguments: `{"command":"printf 'ok'; printf 'fail' >&2; exit 7"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if got.Stdout != "ok" {
		t.Fatalf("Execute().Stdout = %q, want %q", got.Stdout, "ok")
	}
	if got.Stderr != "fail" {
		t.Fatalf("Execute().Stderr = %q, want %q", got.Stderr, "fail")
	}
	if got.ExitCode != 7 {
		t.Fatalf("Execute().ExitCode = %d, want %d", got.ExitCode, 7)
	}
}

type stubWebSearcher struct {
	gotQuery          string
	gotLimit          int
	gotIncludeContent bool
	results           []WebSearchResult
	err               error
}

func (s *stubWebSearcher) Search(ctx context.Context, query string, limit int, includeContent bool) ([]WebSearchResult, error) {
	s.gotQuery = query
	s.gotLimit = limit
	s.gotIncludeContent = includeContent
	if s.err != nil {
		return nil, s.err
	}

	return s.results, nil
}

func TestNewBuiltinExecutorWithExtraHandlersUsesInjectedHandler(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutorWithExtraHandlers(
		[]Definition{
			{
				Name: ToolAgent,
				Kind: KindAgent,
			},
		},
		t.TempDir(),
		map[string]HandlerFunc{
			ToolAgent: func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
				return runtime.ToolExecution{
					Call:   call,
					Output: "delegated",
				}, nil
			},
		},
	)

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolAgent,
		Arguments: `{"description":"review","prompt":"check tests","subagent_name":"reviewer"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "delegated" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "delegated")
	}
}

func TestNewBuiltinExecutorThinkLogsThought(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolThink,
			Kind: KindUtility,
		},
	}, t.TempDir())

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolThink,
		Arguments: `{"thought":"need to compare the parser branches"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "Thought logged" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "Thought logged")
	}
}

func TestNewBuiltinExecutorThinkRejectsEmptyThought(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolThink,
			Kind: KindUtility,
		},
	}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolThink,
		Arguments: `{"thought":"   "}`,
	})
	if !errors.Is(err, ErrToolThoughtRequired) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolThoughtRequired)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestNewSearchWebHandlerUsesInjectedSearcher(t *testing.T) {
	ctx := context.Background()
	searcher := &stubWebSearcher{
		results: []WebSearchResult{
			{
				Title:   "DuckDuckGo result",
				URL:     "https://example.com/result",
				Snippet: "Relevant summary",
				Content: "Fetched page content",
			},
		},
	}
	handler := NewSearchWebHandler(searcher, NewOutputShaperWithLimits(1000, 1000))

	got, err := handler(ctx, runtime.ToolCall{
		Name:      ToolSearchWeb,
		Arguments: `{"query":"duckduckgo api","limit":3,"include_content":true}`,
	}, Definition{Name: ToolSearchWeb, Kind: KindUtility})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if searcher.gotQuery != "duckduckgo api" {
		t.Fatalf("searcher query = %q, want %q", searcher.gotQuery, "duckduckgo api")
	}
	if searcher.gotLimit != 3 {
		t.Fatalf("searcher limit = %d, want %d", searcher.gotLimit, 3)
	}
	if !searcher.gotIncludeContent {
		t.Fatalf("searcher includeContent = false, want true")
	}
	for _, want := range []string{"DuckDuckGo result", "URL: https://example.com/result", "Snippet: Relevant summary", "Content: Fetched page content"} {
		if !strings.Contains(got.Output, want) {
			t.Fatalf("handler output %q missing %q", got.Output, want)
		}
	}
}

func TestDecodeSearchWebArgumentsDefaultsLimit(t *testing.T) {
	got, err := decodeSearchWebArguments(`{"query":" latest go release "}`)
	if err != nil {
		t.Fatalf("decodeSearchWebArguments() error = %v", err)
	}
	if got.Query != "latest go release" {
		t.Fatalf("Query = %q, want %q", got.Query, "latest go release")
	}
	if got.Limit != 5 {
		t.Fatalf("Limit = %d, want %d", got.Limit, 5)
	}
	if got.IncludeContent {
		t.Fatalf("IncludeContent = true, want false")
	}
}

func TestDecodeSearchWebArgumentsRejectsInvalidLimit(t *testing.T) {
	_, err := decodeSearchWebArguments(`{"query":"go","limit":21}`)
	if !errors.Is(err, ErrToolSearchLimitInvalid) {
		t.Fatalf("decodeSearchWebArguments() error = %v, want wrapped %v", err, ErrToolSearchLimitInvalid)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestNewFetchURLHandlerUsesInjectedFetcher(t *testing.T) {
	ctx := context.Background()
	fetcher := &stubURLFetcher{
		content: "Example page content",
	}
	handler := NewFetchURLHandler(fetcher, NewOutputShaperWithLimits(1000, 1000))

	got, err := handler(ctx, runtime.ToolCall{
		Name:      ToolFetchURL,
		Arguments: `{"url":"https://example.com/page"}`,
	}, Definition{Name: ToolFetchURL, Kind: KindUtility})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if fetcher.gotURL != "https://example.com/page" {
		t.Fatalf("fetcher url = %q, want %q", fetcher.gotURL, "https://example.com/page")
	}
	if !strings.Contains(got.Output, "Example page content") {
		t.Fatalf("handler output %q missing %q", got.Output, "Example page content")
	}
}

func TestNewFetchURLHandlerReturnsErrorOnEmptyURL(t *testing.T) {
	ctx := context.Background()
	fetcher := &stubURLFetcher{}
	handler := NewFetchURLHandler(fetcher, NewOutputShaper())

	_, err := handler(ctx, runtime.ToolCall{
		Name:      ToolFetchURL,
		Arguments: `{"url":"   "}`,
	}, Definition{Name: ToolFetchURL, Kind: KindUtility})
	if !errors.Is(err, ErrToolURLRequired) {
		t.Fatalf("handler() error = %v, want wrapped %v", err, ErrToolURLRequired)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestNewFetchURLHandlerReturnsErrorOnFetcherError(t *testing.T) {
	ctx := context.Background()
	fetcher := &stubURLFetcher{
		err: errors.New("network timeout"),
	}
	handler := NewFetchURLHandler(fetcher, NewOutputShaper())

	_, err := handler(ctx, runtime.ToolCall{
		Name:      ToolFetchURL,
		Arguments: `{"url":"https://example.com/timeout"}`,
	}, Definition{Name: ToolFetchURL, Kind: KindUtility})
	if err == nil {
		t.Fatalf("handler() error = nil, want error")
	}
	if !runtime.IsTemporary(err) {
		t.Fatalf("runtime.IsTemporary(error) = false, want true")
	}
}

type stubURLFetcher struct {
	gotURL  string
	content string
	err     error
}

func (s *stubURLFetcher) Fetch(ctx context.Context, url string) (string, error) {
	s.gotURL = url
	if s.err != nil {
		return "", s.err
	}
	return s.content, nil
}

func TestNewBuiltinExecutorSetTodoListRendersTodos(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolSetTodoList,
			Kind: KindUtility,
		},
	}, t.TempDir())

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolSetTodoList,
		Arguments: `{"todos":[{"title":"Inspect runtime loop","status":"Done"},{"title":"Add todo tool","status":"In Progress"}]}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "- Inspect runtime loop [Done]\n- Add todo tool [In Progress]\n" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "- Inspect runtime loop [Done]\n- Add todo tool [In Progress]\n")
	}
}

func TestNewBuiltinExecutorSetTodoListRejectsInvalidStatus(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolSetTodoList,
			Kind: KindUtility,
		},
	}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolSetTodoList,
		Arguments: `{"todos":[{"title":"Inspect runtime loop","status":"Started"}]}`,
	})
	if !errors.Is(err, ErrToolTodoStatusInvalid) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolTodoStatusInvalid)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestNewBashHandlerWithTimeoutCancelsLongRunningCommand(t *testing.T) {
	ctx := context.Background()
	shaper := NewOutputShaperWithLimits(1000, 500) // 使用宽松的限制便于测试
	handler := newBashHandlerWithTimeout(t.TempDir(), shaper, 20*time.Millisecond)

	start := time.Now()
	_, err := handler(ctx, runtime.ToolCall{
		Name:      ToolBash,
		Arguments: `{"command":"sleep 1"}`,
	}, Definition{Name: ToolBash, Kind: KindCommand})
	if !errors.Is(err, ErrToolCommandTimedOut) {
		t.Fatalf("handler() error = %v, want wrapped %v", err, ErrToolCommandTimedOut)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("handler() error = %v, want tool-level timeout error instead of raw context error", err)
	}
	if runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = true, want false")
	}
	if !runtime.IsTemporary(err) {
		t.Fatalf("runtime.IsTemporary(error) = false, want true")
	}
	if time.Since(start) >= 500*time.Millisecond {
		t.Fatalf("handler() duration = %v, want less than %v", time.Since(start), 500*time.Millisecond)
	}
}

func TestNewBuiltinExecutorGlobMatchesWorkspacePaths(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "internal", "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "internal", "tools", "builtin.go"), []byte("package tools\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(builtin.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "internal", "tools", "executor.go"), []byte("package tools\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(executor.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{{
		Name: ToolGlob,
		Kind: KindFile,
	}}, workDir)

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolGlob,
		Arguments: `{"pattern":"internal/**/*.go"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "internal/tools/builtin.go\ninternal/tools/executor.go" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "internal/tools/builtin.go\ninternal/tools/executor.go")
	}
}

func TestNewBuiltinExecutorGlobRejectsPatternOutsideWorkspace(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{{
		Name: ToolGlob,
		Kind: KindFile,
	}}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolGlob,
		Arguments: `{"pattern":"../*.go"}`,
	})
	if !errors.Is(err, ErrToolPatternOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPatternOutsideWorkspace)
	}
}

func TestNewBuiltinExecutorGrepSearchesDirectoryRecursively(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "internal", "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "internal", "tools", "builtin.go"), []byte("type ToolExecution struct{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(builtin.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "internal", "tools", "executor.go"), []byte("type Executor struct{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(executor.go) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{{
		Name: ToolGrep,
		Kind: KindFile,
	}}, workDir)

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolGrep,
		Arguments: `{"pattern":"ToolExecution","path":"internal"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "internal/tools/builtin.go:1:type ToolExecution struct{}" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "internal/tools/builtin.go:1:type ToolExecution struct{}")
	}
}

func TestNewBuiltinExecutorGrepRejectsPathOutsideWorkspace(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{{
		Name: ToolGrep,
		Kind: KindFile,
	}}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolGrep,
		Arguments: `{"pattern":"ToolExecution","path":"../secret"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
	if runtime.IsTemporary(err) {
		t.Fatalf("runtime.IsTemporary(error) = true, want false")
	}
}

func TestNewBuiltinExecutorGrepRejectsEmptyPattern(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolGrep,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolGrep,
		Arguments: `{"pattern":"   "}`,
	})
	if !errors.Is(err, ErrToolPatternRequired) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPatternRequired)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestNewBuiltinExecutorGrepRejectsInvalidRegexPattern(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "notes.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolGrep,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolGrep,
		Arguments: `{"pattern":"(","path":"notes.txt"}`,
	})
	if !errors.Is(err, ErrToolArgumentsInvalid) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolArgumentsInvalid)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
}

func TestNewBuiltinExecutorWriteFileCreatesParentDirectories(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolWriteFile,
			Kind: KindFile,
		},
	}, workDir)

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolWriteFile,
		Arguments: `{"path":"notes/output.txt","content":"hello writer"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "wrote 12 bytes to notes/output.txt" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "wrote 12 bytes to notes/output.txt")
	}

	data, err := os.ReadFile(filepath.Join(workDir, "notes", "output.txt"))
	if err != nil {
		t.Fatalf("ReadFile(output.txt) error = %v", err)
	}
	if string(data) != "hello writer" {
		t.Fatalf("written content = %q, want %q", string(data), "hello writer")
	}
}

func TestNewBuiltinExecutorWriteFileOverwritesExistingFile(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolWriteFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolWriteFile,
		Arguments: `{"path":"notes.txt","content":"new content"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile(notes.txt) error = %v", err)
	}
	if string(data) != "new content" {
		t.Fatalf("written content = %q, want %q", string(data), "new content")
	}
}

func TestNewBuiltinExecutorWriteFileRejectsPathOutsideWorkspace(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolWriteFile,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolWriteFile,
		Arguments: `{"path":"../secret.txt","content":"x"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
}

func TestNewBuiltinExecutorReplaceFileReplacesSingleOccurrence(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("hello old world"), 0o640); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, workDir)

	got, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"old","new":"new"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "replaced 1 occurrence in notes.txt" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "replaced 1 occurrence in notes.txt")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile(notes.txt) error = %v", err)
	}
	if string(data) != "hello new world" {
		t.Fatalf("replaced content = %q, want %q", string(data), "hello new world")
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Stat(notes.txt) error = %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("notes.txt mode = %#o, want %#o", info.Mode().Perm(), 0o640)
	}
}

func TestNewBuiltinExecutorReplaceFileReturnsErrorWhenTargetMissing(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"missing","new":"new"}`,
	})
	if !errors.Is(err, ErrToolReplaceTargetMissing) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolReplaceTargetMissing)
	}
	if !runtime.IsRefused(err) {
		t.Fatalf("runtime.IsRefused(error) = false, want true")
	}
	if runtime.IsTemporary(err) {
		t.Fatalf("runtime.IsTemporary(error) = true, want false")
	}
}

func TestNewBuiltinExecutorReplaceFileReturnsErrorWhenTargetNotUnique(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("old and old"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"old","new":"new"}`,
	})
	if !errors.Is(err, ErrToolReplaceTargetNotUnique) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolReplaceTargetNotUnique)
	}
}

func TestNewBuiltinExecutorReplaceFileRejectsEmptyOldText(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"","new":"new"}`,
	})
	if !errors.Is(err, ErrToolReplaceOldRequired) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolReplaceOldRequired)
	}
}

func TestNewBuiltinExecutorReplaceFileRejectsPathOutsideWorkspace(t *testing.T) {
	ctx := context.Background()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(ctx, runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"../secret.txt","old":"x","new":"y"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
}
