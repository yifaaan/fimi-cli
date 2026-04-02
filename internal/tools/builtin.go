package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"fimi-cli/internal/runtime"
)

var ErrToolArgumentsInvalid = errors.New("tool arguments are invalid")
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
			Call:          call,
			Output:        builder.String(),
			DisplayOutput: strings.TrimSpace(builder.String()),
		}, nil
	}
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

func isAllowedTodoStatus(status string) bool {
	switch status {
	case "Pending", "In Progress", "Done":
		return true
	default:
		return false
	}
}
