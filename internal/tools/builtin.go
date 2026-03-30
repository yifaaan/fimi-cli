package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)

const (
	DefaultBashCommandTimeout    = 120 * time.Second
	MaxBashCommandTimeout        = 300 * time.Second
	DefaultBashBackgroundTimeout = 24 * time.Hour
)

var ErrToolArgumentsInvalid = errors.New("tool arguments are invalid")
var ErrToolCommandRequired = errors.New("tool command is required")
var ErrToolCommandTimedOut = errors.New("tool command timed out")
var ErrToolPathRequired = errors.New("tool path is required")
var ErrToolPatternRequired = errors.New("tool pattern is required")
var ErrToolSearchQueryRequired = errors.New("tool search query is required")
var ErrToolSearchLimitInvalid = errors.New("tool search limit is invalid")
var ErrToolReplaceOldRequired = errors.New("tool replace old text is required")
var ErrToolReplaceTargetMissing = errors.New("tool replace target not found")
var ErrToolReplaceTargetNotUnique = errors.New("tool replace target is not unique")
var ErrToolPathOutsideWorkspace = errors.New("tool path escapes workspace")
var ErrToolPatternOutsideWorkspace = errors.New("tool pattern escapes workspace")
var ErrToolPatchDiffRequired = errors.New("tool patch diff is required")
var ErrToolPatchFailed = errors.New("failed to apply patch")
var ErrToolThoughtRequired = errors.New("tool thought is required")
var ErrToolTodosRequired = errors.New("tool todos are required")
var ErrToolTodoTitleRequired = errors.New("tool todo title is required")
var ErrToolTodoStatusInvalid = errors.New("tool todo status is invalid")
var ErrToolURLRequired = errors.New("tool url is required")

type bashArguments struct {
	Command    string `json:"command"`
	Timeout    int    `json:"timeout"`    // 秒，0 = 使用默认值，最大 300
	Background bool   `json:"background"` // 为 true 时后台运行，立即返回 task ID
	TaskID     string `json:"task_id"`    // 非空时查询指定后台任务的状态
}

type thinkArguments struct {
	Thought string `json:"thought"`
}

type todoItemArguments struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type setTodoListArguments struct {
	Todos []todoItemArguments `json:"todos"`
}

type searchWebArguments struct {
	Query          string `json:"query"`
	Limit          int    `json:"limit"`
	IncludeContent bool   `json:"include_content"`
}

type fetchURLArguments struct {
	URL string `json:"url"`
}

type URLFetcher interface {
	Fetch(ctx context.Context, url string) (string, error)
}

type WebSearchResult struct {
	Title   string
	URL     string
	Snippet string
	Content string
}

type WebSearcher interface {
	Search(ctx context.Context, query string, limit int, includeContent bool) ([]WebSearchResult, error)
}

type globArguments struct {
	Pattern string `json:"pattern"`
}

type grepArguments struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

type readFileArguments struct {
	Path string `json:"path"`
}

type writeFileArguments struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type replaceFileArguments struct {
	Path       string `json:"path"`
	Old        string `json:"old"`
	New        string `json:"new"`
	ReplaceAll bool   `json:"replace_all"` // 为 true 时替换所有匹配，否则要求恰好匹配一次
}

type patchFileArguments struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

// ExecutorOption 配置 BuiltinExecutor 的可选参数。
type ExecutorOption func(*executorOpts)

type executorOpts struct {
	shaper        OutputShaper
	extraHandlers map[string]HandlerFunc
}

// WithShaper 设置自定义输出塑形器，默认使用 NewOutputShaper()。
func WithShaper(shaper OutputShaper) ExecutorOption {
	return func(o *executorOpts) { o.shaper = shaper }
}

// WithExtraHandlers 追加自定义工具 handler，覆盖同名内建 handler。
func WithExtraHandlers(handlers map[string]HandlerFunc) ExecutorOption {
	return func(o *executorOpts) { o.extraHandlers = handlers }
}

// NewBuiltinExecutor 返回带内建 handler 的工具执行器。
// 通过 ExecutorOption 可选地注入自定义塑形器或额外 handler。
func NewBuiltinExecutor(definitions []Definition, workDir string, bgMgr *BackgroundManager, opts ...ExecutorOption) Executor {
	o := executorOpts{
		shaper: NewOutputShaper(),
	}
	for _, opt := range opts {
		opt(&o)
	}

	handlers := builtinHandlers(workDir, o.shaper, bgMgr)
	maps.Copy(handlers, o.extraHandlers)

	return NewExecutor(definitions, handlers)
}

func builtinHandlers(workDir string, shaper OutputShaper, bgMgr *BackgroundManager) map[string]HandlerFunc {
	return map[string]HandlerFunc{
		ToolThink:       newThinkHandler(),
		ToolSetTodoList: newSetTodoListHandler(),
		ToolBash:        newBashHandler(workDir, shaper, bgMgr),
		ToolGlob:        newGlobHandler(workDir, shaper),
		ToolGrep:        newGrepHandler(workDir, shaper),
		ToolReadFile:    newReadFileHandler(workDir, shaper),
		ToolWriteFile:   newWriteFileHandler(workDir),
		ToolReplaceFile: newReplaceFileHandler(workDir),
		ToolPatchFile:   newPatchFileHandler(workDir),
	}
}

func newThinkHandler() HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeThinkArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		_ = args

		return runtime.ToolExecution{
			Call:   call,
			Output: "Thought logged",
		}, nil
	}
}

func newSetTodoListHandler() HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeSetTodoListArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		var builder strings.Builder
		for _, todo := range args.Todos {
			builder.WriteString("- ")
			builder.WriteString(todo.Title)
			builder.WriteString(" [")
			builder.WriteString(todo.Status)
			builder.WriteString("]\n")
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: builder.String(),
		}, nil
	}
}

func newBashHandler(workDir string, shaper OutputShaper, bgMgr *BackgroundManager) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// Approval gate
		if a := approval.FromContext(ctx); a != nil {
			desc := args.Command
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			if err := a.Request(ctx, "bash", desc); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		// 模式 1：查询后台任务状态
		if args.TaskID != "" {
			return handleBashTaskQuery(call, args, bgMgr)
		}

		// 模式 2：后台执行
		if args.Background {
			return handleBashBackground(call, args, workDir, bgMgr)
		}

		// 模式 3：前台执行（原有逻辑）
		return handleBashForeground(ctx, call, args, workDir, shaper)
	}
}

// handleBashTaskQuery 查询后台任务状态并格式化输出。
func handleBashTaskQuery(call runtime.ToolCall, args bashArguments, bgMgr *BackgroundManager) (runtime.ToolExecution, error) {
	if bgMgr == nil {
		return runtime.ToolExecution{}, markRefused(fmt.Errorf("background task manager not available"))
	}

	result, err := bgMgr.Status(args.TaskID)
	if err != nil {
		return runtime.ToolExecution{}, markRefused(err)
	}

	var outputParts []string
	outputParts = append(outputParts, fmt.Sprintf("Task %s [%s]", result.ID, result.Status))
	outputParts = append(outputParts, fmt.Sprintf("Command: %s", result.Command))
	outputParts = append(outputParts, fmt.Sprintf("Duration: %s", result.Duration.Round(time.Millisecond)))
	if result.ExitCode != 0 {
		outputParts = append(outputParts, fmt.Sprintf("Exit code: %d", result.ExitCode))
	}
	if result.Stdout != "" {
		outputParts = append(outputParts, "STDOUT:", result.Stdout)
	}
	if result.Stderr != "" {
		outputParts = append(outputParts, "STDERR:", result.Stderr)
	}

	return runtime.ToolExecution{
		Call:   call,
		Output: strings.Join(outputParts, "\n"),
	}, nil
}

// handleBashBackground 在后台启动命令，立即返回任务 ID。
func handleBashBackground(call runtime.ToolCall, args bashArguments, workDir string, bgMgr *BackgroundManager) (runtime.ToolExecution, error) {
	if bgMgr == nil {
		return runtime.ToolExecution{}, markRefused(fmt.Errorf("background task manager not available"))
	}
	if strings.TrimSpace(args.Command) == "" {
		return runtime.ToolExecution{}, markRefused(ErrToolCommandRequired)
	}

	taskID, err := bgMgr.Start(args.Command, workDir, 0)
	if err != nil {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("start background task: %w", err))
	}

	return runtime.ToolExecution{
		Call:   call,
		Output: fmt.Sprintf("Background task started: %s (use task_id=\"%s\" to check status)", taskID, taskID),
	}, nil
}

// handleBashForeground 是原有的同步前台执行逻辑。
func handleBashForeground(ctx context.Context, call runtime.ToolCall, args bashArguments, workDir string, shaper OutputShaper) (runtime.ToolExecution, error) {
	// 从参数计算超时：0 → 默认值，超过上限则截断
	timeout := DefaultBashCommandTimeout
	if args.Timeout > 0 {
		timeout = min(time.Duration(args.Timeout)*time.Second, MaxBashCommandTimeout)
	}

	// 使用传入的 ctx 作为父 context，这样外部取消也能中断 bash 执行
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = workDir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command))
	}
	if err != nil && !isExitError(err) {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("run bash command: %w", err))
	}

	// 对 stdout 进行塑形
	rawStdout := stdout.String()
	shapedStdout := shaper.Shape(rawStdout)

	// 对 stderr 进行塑形（使用相同的限制）
	rawStderr := stderr.String()
	shapedStderr := shaper.Shape(rawStderr)

	// 构建最终输出：stdout + stderr + 截断提示
	var outputParts []string
	outputParts = append(outputParts, shapedStdout.Output)
	if shapedStderr.Output != "" {
		outputParts = append(outputParts, "STDERR:", shapedStderr.Output)
	}

	// 添加截断提示
	var truncationMsgs []string
	if shapedStdout.Message != "" {
		truncationMsgs = append(truncationMsgs, "stdout: "+shapedStdout.Message)
	}
	if shapedStderr.Message != "" {
		truncationMsgs = append(truncationMsgs, "stderr: "+shapedStderr.Message)
	}
	if len(truncationMsgs) > 0 {
		outputParts = append(outputParts, "\n["+strings.Join(truncationMsgs, "; ")+"]")
	}

	return runtime.ToolExecution{
		Call:     call,
		Output:   strings.Join(outputParts, "\n"),
		Stdout:   shapedStdout.Output,
		Stderr:   shapedStderr.Output,
		ExitCode: exitCodeFromError(err),
	}, nil
}

// newBashHandlerWithTimeout 提供固定超时的 bash handler，仅用于测试。
func newBashHandlerWithTimeout(workDir string, shaper OutputShaper, timeout time.Duration) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
		if workDir != "" {
			cmd.Dir = workDir
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return runtime.ToolExecution{}, markTemporary(fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command))
			}
			if !isExitError(err) {
				return runtime.ToolExecution{}, markTemporary(fmt.Errorf("run bash command: %w", err))
			}
		}

		shapedStdout := shaper.Shape(stdout.String())
		shapedStderr := shaper.Shape(stderr.String())

		return runtime.ToolExecution{
			Call:     call,
			Output:   shapedStdout.Output,
			Stdout:   shapedStdout.Output,
			Stderr:   shapedStderr.Output,
			ExitCode: exitCodeFromError(err),
		}, nil
	}
}

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

		// 对文件内容进行塑形
		rawContent := string(data)
		shaped := shaper.Shape(rawContent)

		// 构建最终输出
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
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

		rawOutput := strings.Join(matches, "\n")
		shaped := shaper.Shape(rawOutput)
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
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

		rawOutput := strings.Join(matches, "\n")
		shaped := shaper.Shape(rawOutput)
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
		}, nil
	}
}

func newWriteFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeWriteFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// Approval gate
		if a := approval.FromContext(ctx); a != nil {
			if err := a.Request(ctx, "write_file", args.Path); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetRel, err := relativeWorkspacePath(rootAbs, targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("create parent dir for %q: %w", targetAbs, err)
		}
		if err := os.WriteFile(targetAbs, []byte(args.Content), 0o644); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: fmt.Sprintf("wrote %d bytes to %s", len([]byte(args.Content)), targetRel),
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
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
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
		output := shaped.Output
		if shaped.Message != "" {
			output += "\n\n[" + shaped.Message + "]"
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: output,
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

func newReplaceFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeReplaceFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		// Approval gate
		if a := approval.FromContext(ctx); a != nil {
			if err := a.Request(ctx, "replace_file", args.Path); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetRel, err := relativeWorkspacePath(rootAbs, targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		data, err := os.ReadFile(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("read file %q for replace: %w", targetAbs, err)
		}
		info, err := os.Stat(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("stat file %q for replace: %w", targetAbs, err)
		}

		content := string(data)
		matchCount := strings.Count(content, args.Old)

		if matchCount == 0 {
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %s", ErrToolReplaceTargetMissing, targetRel))
		}

		var replaced string
		var outputMsg string

		if args.ReplaceAll {
			// replace_all 模式：替换所有匹配项
			replaced = strings.ReplaceAll(content, args.Old, args.New)
			outputMsg = fmt.Sprintf("replaced %d occurrence(s) in %s", matchCount, targetRel)
		} else {
			// 单次替换模式：要求恰好匹配一次
			if matchCount > 1 {
				return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %s (found %d, set replace_all to true to replace all)", ErrToolReplaceTargetNotUnique, targetRel, matchCount))
			}
			replaced = strings.Replace(content, args.Old, args.New, 1)
			outputMsg = fmt.Sprintf("replaced 1 occurrence in %s", targetRel)
		}

		if err := os.WriteFile(targetAbs, []byte(replaced), info.Mode().Perm()); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write replaced file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: outputMsg,
		}, nil
	}
}

func newPatchFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodePatchFileArguments(call.Arguments)
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
		targetRel, err := relativeWorkspacePath(rootAbs, targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		info, err := os.Stat(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("stat file %q for patch: %w", targetAbs, err)
		}

		originalData, err := os.ReadFile(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("read file %q for patch: %w", targetAbs, err)
		}

		patchedContent, hunksApplied, err := applyUnifiedDiff(string(originalData), args.Diff)
		if err != nil {
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %v", ErrToolPatchFailed, err))
		}

		if patchedContent == string(originalData) {
			return runtime.ToolExecution{}, markRefused(errors.New("no changes were made by the patch"))
		}

		if err := os.WriteFile(targetAbs, []byte(patchedContent), info.Mode().Perm()); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write patched file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: fmt.Sprintf("applied %d hunk(s) to %s", hunksApplied, targetRel),
		}, nil
	}
}

func decodeBashArguments(raw string) (bashArguments, error) {
	var args bashArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return bashArguments{}, markRefused(fmt.Errorf("%w: decode bash arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Command) == "" {
		return bashArguments{}, markRefused(ErrToolCommandRequired)
	}

	return args, nil
}

func decodeThinkArguments(raw string) (thinkArguments, error) {
	var args thinkArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return thinkArguments{}, markRefused(fmt.Errorf("%w: decode think arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Thought) == "" {
		return thinkArguments{}, markRefused(ErrToolThoughtRequired)
	}

	return args, nil
}

func decodeSetTodoListArguments(raw string) (setTodoListArguments, error) {
	var args setTodoListArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return setTodoListArguments{}, markRefused(fmt.Errorf("%w: decode set_todo_list arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if len(args.Todos) == 0 {
		return setTodoListArguments{}, markRefused(ErrToolTodosRequired)
	}

	for i := range args.Todos {
		args.Todos[i].Title = strings.TrimSpace(args.Todos[i].Title)
		args.Todos[i].Status = strings.TrimSpace(args.Todos[i].Status)

		if args.Todos[i].Title == "" {
			return setTodoListArguments{}, markRefused(ErrToolTodoTitleRequired)
		}
		if !isAllowedTodoStatus(args.Todos[i].Status) {
			return setTodoListArguments{}, markRefused(fmt.Errorf("%w: %s", ErrToolTodoStatusInvalid, args.Todos[i].Status))
		}
	}

	return args, nil
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

func decodeWriteFileArguments(raw string) (writeFileArguments, error) {
	var args writeFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return writeFileArguments{}, markRefused(fmt.Errorf("%w: decode write_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return writeFileArguments{}, markRefused(ErrToolPathRequired)
	}

	return args, nil
}

func decodeReplaceFileArguments(raw string) (replaceFileArguments, error) {
	var args replaceFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return replaceFileArguments{}, markRefused(fmt.Errorf("%w: decode replace_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return replaceFileArguments{}, markRefused(ErrToolPathRequired)
	}
	if args.Old == "" {
		return replaceFileArguments{}, markRefused(ErrToolReplaceOldRequired)
	}

	return args, nil
}

func decodePatchFileArguments(raw string) (patchFileArguments, error) {
	var args patchFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return patchFileArguments{}, markRefused(fmt.Errorf("%w: decode patch_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return patchFileArguments{}, markRefused(ErrToolPathRequired)
	}
	if strings.TrimSpace(args.Diff) == "" {
		return patchFileArguments{}, markRefused(ErrToolPatchDiffRequired)
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

func isExitError(err error) bool {
	if err == nil {
		return false
	}

	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
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

func isAllowedTodoStatus(status string) bool {
	switch status {
	case "Pending", "In Progress", "Done":
		return true
	default:
		return false
	}
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

// applyUnifiedDiff 将 unified diff 应用到原始内容上。
// 返回修改后的内容、应用的 hunk 数量、以及可能的错误。
func applyUnifiedDiff(original string, diff string) (string, int, error) {
	// 解析 diff 为 hunk 列表
	hunks, err := parseUnifiedDiff(diff)
	if err != nil {
		return "", 0, err
	}

	if len(hunks) == 0 {
		return "", 0, errors.New("no valid hunks found in diff")
	}

	// 将原始内容按行分割（保留换行符信息）
	lines := splitLinesKeepEnds(original)

	// 从后向前应用 hunk，避免行号偏移问题
	for i := len(hunks) - 1; i >= 0; i-- {
		hunk := hunks[i]
		lines, err = applyHunk(lines, hunk)
		if err != nil {
			return "", 0, fmt.Errorf("apply hunk %d: %w", i+1, err)
		}
	}

	result := strings.Join(lines, "")
	return result, len(hunks), nil
}

// unifiedHunk 表示一个 unified diff hunk
type unifiedHunk struct {
	oldStart int      // 旧文件起始行号（1-based）
	oldCount int      // 旧文件行数
	newStart int      // 新文件起始行号（1-based）
	newCount int      // 新文件行数
	lines    []string // hunk 中的所有行（包含前缀字符）
}

// parseUnifiedDiff 解析 unified diff 格式的字符串
func parseUnifiedDiff(diff string) ([]unifiedHunk, error) {
	diffLines := strings.Split(diff, "\n")

	var hunks []unifiedHunk
	var currentHunk *unifiedHunk

	inHunk := false

	for _, line := range diffLines {
		// 跳过文件头
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}

		// 检测 hunk 头
		if strings.HasPrefix(line, "@@") {
			// 保存之前的 hunk
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}

			// 解析 hunk 头：@@ -oldStart,oldCount +newStart,newCount @@
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			currentHunk = &hunk
			inHunk = true
			continue
		}

		// 收集 hunk 内容
		if inHunk && currentHunk != nil {
			// 只收集有意义的行（以空格、-、+ 开头的行）
			if len(line) > 0 {
				prefix := line[0]
				if prefix == ' ' || prefix == '-' || prefix == '+' {
					currentHunk.lines = append(currentHunk.lines, line)
				}
			}
		}
	}

	// 保存最后一个 hunk
	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks, nil
}

// parseHunkHeader 解析 hunk 头：@@ -oldStart,oldCount +newStart,newCount @@
func parseHunkHeader(line string) (unifiedHunk, error) {
	// 格式：@@ -start,count +start,count @@ optional text
	re := regexp.MustCompile(`@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return unifiedHunk{}, errors.New("invalid hunk header format")
	}

	oldStart, _ := strconv.Atoi(matches[1])
	oldCount := 1
	if matches[2] != "" {
		oldCount, _ = strconv.Atoi(matches[2])
	}

	newStart, _ := strconv.Atoi(matches[3])
	newCount := 1
	if matches[4] != "" {
		newCount, _ = strconv.Atoi(matches[4])
	}

	return unifiedHunk{
		oldStart: oldStart,
		oldCount: oldCount,
		newStart: newStart,
		newCount: newCount,
		lines:    make([]string, 0),
	}, nil
}

// applyHunk 将单个 hunk 应用到行列表
func applyHunk(lines []string, hunk unifiedHunk) ([]string, error) {
	// 转换为 0-based 索引
	startIdx := hunk.oldStart - 1

	// 验证起始位置
	if startIdx < 0 || startIdx > len(lines) {
		return nil, fmt.Errorf("invalid old start line %d (file has %d lines)", hunk.oldStart, len(lines))
	}

	// 验证上下文行匹配
	contextIdx := startIdx
	for _, hunkLine := range hunk.lines {
		prefix := hunkLine[0]
		content := hunkLine[1:]

		if prefix == ' ' || prefix == '-' {
			// 验证上下文和删除行是否匹配
			if contextIdx >= len(lines) {
				return nil, fmt.Errorf("unexpected end of file at line %d", contextIdx+1)
			}

			// 比较：diff 中可能没有换行符，但文件行保留原始换行符
			// 需要两边都去掉换行符后再比较
			expected := strings.TrimRight(content, "\r\n")
			actual := strings.TrimRight(lines[contextIdx], "\r\n")
			if expected != actual {
				return nil, fmt.Errorf("line %d mismatch: expected %q, got %q", contextIdx+1, expected, actual)
			}
			contextIdx++
		}
	}

	// 构建新的行列表
	result := make([]string, 0, len(lines))
	result = append(result, lines[:startIdx]...) // 前面的行保持不变

	// 处理 hunk 内容
	for _, hunkLine := range hunk.lines {
		prefix := hunkLine[0]
		content := hunkLine[1:]

		switch prefix {
		case ' ':
			// 上下文行：保留原行（包括换行符）
			result = append(result, lines[startIdx])
			startIdx++
		case '-':
			// 删除行：跳过原行
			startIdx++
		case '+':
			// 添加行：插入新行
			// 如果内容没有换行符，添加一个
			if !strings.HasSuffix(content, "\n") && !strings.HasSuffix(content, "\r") {
				content += "\n"
			}
			result = append(result, content)
		}
	}

	// 添加剩余的行
	result = append(result, lines[startIdx:]...)

	return result, nil
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
	// 允许较长源码行，避免默认 64K 上限过早打断搜索。
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
