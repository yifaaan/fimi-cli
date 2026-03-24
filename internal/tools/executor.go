package tools

import (
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/runtime"
)

var ErrToolCallNameRequired = errors.New("tool call name is required")
var ErrToolCallNotAllowed = errors.New("tool call is not allowed")

// HandlerFunc 定义单个工具名对应的执行逻辑。
// 当前先保留最小函数签名，后面再扩展上下文、工作目录等运行参数。
type HandlerFunc func(call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error)

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
func (e Executor) Execute(call runtime.ToolCall) (runtime.ToolExecution, error) {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return runtime.ToolExecution{}, ErrToolCallNameRequired
	}

	definition, ok := e.allowedTools[name]
	if !ok {
		return runtime.ToolExecution{}, fmt.Errorf("%w: %s", ErrToolCallNotAllowed, name)
	}

	call.Name = name
	handler, ok := e.handlers[name]
	if !ok {
		return runtime.ToolExecution{
			Call: call,
		}, nil
	}

	execution, err := handler(call, definition)
	if err != nil {
		return runtime.ToolExecution{}, fmt.Errorf("execute tool %q: %w", name, err)
	}

	return execution, nil
}
