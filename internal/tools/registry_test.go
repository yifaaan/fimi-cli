package tools

import (
	"errors"
	"reflect"
	"testing"
)

func TestRegistryRegisterAndResolve(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	err = registry.Register(Definition{
		Name:        " bash ",
		Kind:        KindCommand,
		Description: " run bash ",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := registry.Resolve("bash")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Definition{
		Name:        "bash",
		Kind:        KindCommand,
		Description: "run bash",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %#v, want %#v", got, want)
	}
}

func TestRegistryResolveAllPreservesOrder(t *testing.T) {
	registry := BuiltinRegistry()

	got, err := registry.ResolveAll([]string{ToolReadFile, ToolBash, ToolWriteFile})
	if err != nil {
		t.Fatalf("ResolveAll() error = %v", err)
	}

	want := []Definition{
		{
			Name:        ToolReadFile,
			Kind:        KindFile,
			Description: "Read a file from the workspace.",
		},
		{
			Name:        ToolBash,
			Kind:        KindCommand,
			Description: "Run a shell command inside the workspace.",
		},
		{
			Name:        ToolWriteFile,
			Kind:        KindFile,
			Description: "Write a file inside the workspace.",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveAll() = %#v, want %#v", got, want)
	}
}

func TestBuiltinRegistryIncludesAgentTool(t *testing.T) {
	registry := BuiltinRegistry()

	got, err := registry.Resolve(ToolAgent)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Name != ToolAgent {
		t.Fatalf("Resolve().Name = %q, want %q", got.Name, ToolAgent)
	}
	if got.Kind != KindAgent {
		t.Fatalf("Resolve().Kind = %q, want %q", got.Kind, KindAgent)
	}
}

func TestRegistryReturnsStructuredErrors(t *testing.T) {
	registry := BuiltinRegistry()

	_, err := registry.Resolve("missing")
	if !errors.Is(err, ErrToolNotRegistered) {
		t.Fatalf("Resolve() error = %v, want wrapped %v", err, ErrToolNotRegistered)
	}

	err = registry.Register(Definition{Name: ToolBash})
	if !errors.Is(err, ErrToolAlreadyRegistered) {
		t.Fatalf("Register() error = %v, want wrapped %v", err, ErrToolAlreadyRegistered)
	}

	err = registry.Register(Definition{})
	if !errors.Is(err, ErrToolNameRequired) {
		t.Fatalf("Register() error = %v, want wrapped %v", err, ErrToolNameRequired)
	}
}
