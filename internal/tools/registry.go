package tools

import (
	"errors"
	"fmt"
	"strings"
)

var ErrToolNameRequired = errors.New("tool name is required")
var ErrToolAlreadyRegistered = errors.New("tool already registered")
var ErrToolNotRegistered = errors.New("tool not registered")

// Kind 描述工具的粗粒度类别。
// 现在先只用于装配边界上的元数据，不承载执行逻辑。
type Kind string

const (
	KindCommand Kind = "command"
	KindFile    Kind = "file"
	KindAgent   Kind = "agent"
	KindUtility Kind = "utility"
)

const (
	ToolBash        = "bash"
	ToolAgent       = "agent"
	ToolThink       = "think"
	ToolSetTodoList = "set_todo_list"
	ToolSearchWeb   = "search_web"
	ToolFetchURL    = "fetch_url"
	ToolReadFile    = "read_file"
	ToolGlob        = "glob"
	ToolGrep        = "grep"
	ToolWriteFile   = "write_file"
	ToolReplaceFile = "replace_file"
	ToolPatchFile   = "patch_file"
)

const setTodoListDescription = `Update the whole todo list.

Todo list is a simple yet powerful tool to help you get things done. You typically want to use this tool when the given task involves multiple subtasks or milestones, or when multiple tasks are given in a single request. This tool helps you break down the task and track progress.

This is the only todo list tool available. Each time you want to operate on the todo list, you need to update the whole list. Make sure to maintain the todo items and their statuses properly.

Once you finish a subtask or milestone, update the todo list to reflect progress.

Avoid using this tool for trivial one-step tasks or simple factual questions.`

// Definition 表示 runtime 未来会消费的单个工具声明。
// 当前阶段先保留稳定的名字、类别和说明。
type Definition struct {
	Name        string
	Kind        Kind
	Description string
}

// Registry 是工具名到定义的显式映射表。
// 这里先解决“agent 声明的工具是否被 Go 运行时认识”的问题。
type Registry struct {
	definitions map[string]Definition
}

// NewRegistry 用给定定义创建一个 registry。
func NewRegistry(definitions ...Definition) (Registry, error) {
	registry := Registry{
		definitions: make(map[string]Definition, len(definitions)),
	}
	for _, definition := range definitions {
		if err := registry.Register(definition); err != nil {
			return Registry{}, err
		}
	}

	return registry, nil
}

// BuiltinRegistry 返回当前内建的最小工具注册表。
func BuiltinRegistry() Registry {
	registry, err := NewRegistry(
		Definition{
			Name:        ToolAgent,
			Kind:        KindAgent,
			Description: "Run a declared subagent for a focused task.",
		},
		Definition{
			Name:        ToolThink,
			Kind:        KindUtility,
			Description: "Log a private reasoning note without changing workspace state.",
		},
		Definition{
			Name:        ToolSetTodoList,
			Kind:        KindUtility,
			Description: setTodoListDescription,
		},
		Definition{
			Name:        ToolBash,
			Kind:        KindCommand,
			Description: "Run a shell command inside the workspace.",
		},
		Definition{
			Name:        ToolSearchWeb,
			Kind:        KindUtility,
			Description: "Search the web for recent information and relevant pages.",
		},
		Definition{
			Name:        ToolFetchURL,
			Kind:        KindUtility,
			Description: "Fetch a URL and return its main text content.",
		},
		Definition{
			Name:        ToolReadFile,
			Kind:        KindFile,
			Description: "Read a file from the workspace.",
		},
		Definition{
			Name:        ToolGlob,
			Kind:        KindFile,
			Description: "List workspace paths that match a glob pattern.",
		},
		Definition{
			Name:        ToolGrep,
			Kind:        KindFile,
			Description: "Search workspace files for matching text.",
		},
		Definition{
			Name:        ToolWriteFile,
			Kind:        KindFile,
			Description: "Write a file inside the workspace.",
		},
		Definition{
			Name:        ToolReplaceFile,
			Kind:        KindFile,
			Description: "Replace text in an existing workspace file.",
		},
		Definition{
			Name:        ToolPatchFile,
			Kind:        KindFile,
			Description: "Apply a unified diff patch to an existing workspace file.",
		},
	)
	if err != nil {
		panic(err)
	}

	return registry
}

// Register 向 registry 中注册一个工具定义。
func (r *Registry) Register(definition Definition) error {
	name := strings.TrimSpace(definition.Name)
	if name == "" {
		return ErrToolNameRequired
	}
	if r.definitions == nil {
		r.definitions = make(map[string]Definition, 1)
	}
	if _, exists := r.definitions[name]; exists {
		return fmt.Errorf("%w: %s", ErrToolAlreadyRegistered, name)
	}

	definition.Name = name
	definition.Description = strings.TrimSpace(definition.Description)
	r.definitions[name] = definition

	return nil
}

// Resolve 按名字查找单个工具定义。
func (r Registry) Resolve(name string) (Definition, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Definition{}, ErrToolNameRequired
	}

	definition, ok := r.definitions[name]
	if !ok {
		return Definition{}, fmt.Errorf("%w: %s", ErrToolNotRegistered, name)
	}

	return definition, nil
}

// ResolveAll 按 agent.yaml 声明顺序解析所有工具。
func (r Registry) ResolveAll(names []string) ([]Definition, error) {
	resolved := make([]Definition, 0, len(names))
	for _, name := range names {
		definition, err := r.Resolve(name)
		if err != nil {
			return nil, err
		}

		resolved = append(resolved, definition)
	}

	return resolved, nil
}
