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
)

const (
	ToolBash        = "bash"
	ToolReadFile    = "read_file"
	ToolGlob        = "glob"
	ToolGrep        = "grep"
	ToolWriteFile   = "write_file"
	ToolReplaceFile = "replace_file"
)

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
			Name:        ToolBash,
			Kind:        KindCommand,
			Description: "Run a shell command inside the workspace.",
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
