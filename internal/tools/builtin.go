package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"strings"
	"time"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)

const (
	DefaultBashCommandTimeout = 120 * time.Second
	MaxBashCommandTimeout     = 300 * time.Second
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
			desc := bashApprovalDescription(args)
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

func decodeBashArguments(raw string) (bashArguments, error) {
	var args bashArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return bashArguments{}, markRefused(fmt.Errorf("%w: decode bash arguments: %v", ErrToolArgumentsInvalid, err))
	}

	args.Command = strings.TrimSpace(args.Command)
	args.TaskID = strings.TrimSpace(args.TaskID)

	if args.TaskID != "" {
		return args, nil
	}
	if args.Command == "" {
		return bashArguments{}, markRefused(ErrToolCommandRequired)
	}

	return args, nil
}

func bashApprovalDescription(args bashArguments) string {
	if args.TaskID != "" {
		return fmt.Sprintf("query background task %s", args.TaskID)
	}
	if args.Background {
		return "background: " + args.Command
	}

	return args.Command
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

func isAllowedTodoStatus(status string) bool {
	switch status {
	case "Pending", "In Progress", "Done":
		return true
	default:
		return false
	}
}
