package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)

var ErrToolCallNameRequired = errors.New("tool call name is required")
var ErrToolCallNotAllowed = errors.New("tool call is not allowed")

type refusedError struct {
	err error
}

func (e refusedError) Error() string {
	return e.err.Error()
}

func (e refusedError) Unwrap() error {
	return e.err
}

func (refusedError) Refused() bool {
	return true
}

type temporaryError struct {
	err error
}

func (e temporaryError) Error() string {
	return e.err.Error()
}

func (e temporaryError) Unwrap() error {
	return e.err
}

func (temporaryError) Temporary() bool {
	return true
}

// HandlerFunc 定义单个工具名对应的执行逻辑。
// ctx 用于超时/取消传播；call 是工具调用参数；definition 是工具定义。
type HandlerFunc func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error)

// Executor 是 runtime.ToolExecutor 在 tools 包内的最小适配器。
// 它负责校验当前 agent 允许调用哪些工具，并把执行委托给对应 handler。
type Executor struct {
	allowedTools map[string]Definition
	handlers     map[string]HandlerFunc
}

// NewExecutor 用 agent 已解析的工具定义构造执行器。
// 如果某个工具没有自定义 handler，就回退到默认 no-op 行为。
func NewExecutor(definitions []Definition, handlers map[string]HandlerFunc) Executor {
	allowedTools := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		allowedTools[definition.Name] = definition
	}

	if handlers == nil {
		handlers = make(map[string]HandlerFunc)
	}

	return Executor{
		allowedTools: allowedTools,
		handlers:     handlers,
	}
}

// Execute 校验工具调用是否被当前 agent 允许，然后执行对应 handler。
func (e Executor) Execute(ctx context.Context, call runtime.ToolCall) (runtime.ToolExecution, error) {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return runtime.ToolExecution{}, markRefused(ErrToolCallNameRequired)
	}

	definition, ok := e.allowedTools[name]
	if !ok {
		return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %s", ErrToolCallNotAllowed, name))
	}

	call.Name = name
	ctx = approval.WithToolCallID(ctx, call.ID)
	handler, ok := e.handlers[name]
	if !ok {
		return runtime.ToolExecution{
			Call: call,
		}, nil
	}

	execution, err := handler(ctx, call, definition)
	if err != nil {
		return runtime.ToolExecution{}, fmt.Errorf("execute tool %q: %w", name, err)
	}

	return execution, nil
}

func markRefused(err error) error {
	if err == nil || runtime.IsRefused(err) {
		return err
	}

	return refusedError{err: err}
}

func markTemporary(err error) error {
	if err == nil || runtime.IsTemporary(err) {
		return err
	}

	return temporaryError{err: err}
}
