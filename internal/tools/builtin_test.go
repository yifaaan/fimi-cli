package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestNewBuiltinExecutorReadFileReadsWorkspaceFile(t *testing.T) {
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

	got, err := executor.Execute(runtime.ToolCall{
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
	workDir := t.TempDir()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReadFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolReadFile,
		Arguments: `{"path":"../secret.txt"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
}

func TestNewBuiltinExecutorBashRunsCommandInsideWorkDir(t *testing.T) {
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

	got, err := executor.Execute(runtime.ToolCall{
		Name:      ToolBash,
		Arguments: `{"command":"printf '%s' \"$PWD\" && printf '\n' && ls marker.txt"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(got.Output), "\n")
	if len(lines) != 2 {
		t.Fatalf("len(output lines) = %d, want %d, output=%q", len(lines), 2, got.Output)
	}
	if lines[0] != workDir {
		t.Fatalf("output workDir line = %q, want %q", lines[0], workDir)
	}
	if lines[1] != "marker.txt" {
		t.Fatalf("output file line = %q, want %q", lines[1], "marker.txt")
	}
}

func TestNewBuiltinExecutorBashReturnsErrorOnNonZeroExit(t *testing.T) {
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolBash,
			Kind: KindCommand,
		},
	}, t.TempDir())

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolBash,
		Arguments: `{"command":"exit 7"}`,
	})
	if err == nil {
		t.Fatalf("Execute() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "run bash command") {
		t.Fatalf("Execute() error = %q, want contains %q", err.Error(), "run bash command")
	}
}

func TestNewBuiltinExecutorGlobMatchesWorkspacePaths(t *testing.T) {
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

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolGlob,
			Kind: KindFile,
		},
	}, workDir)

	got, err := executor.Execute(runtime.ToolCall{
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
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolGlob,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolGlob,
		Arguments: `{"pattern":"../*.go"}`,
	})
	if !errors.Is(err, ErrToolPatternOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPatternOutsideWorkspace)
	}
}

func TestNewBuiltinExecutorGrepSearchesDirectoryRecursively(t *testing.T) {
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

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolGrep,
			Kind: KindFile,
		},
	}, workDir)

	got, err := executor.Execute(runtime.ToolCall{
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
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolGrep,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolGrep,
		Arguments: `{"pattern":"ToolExecution","path":"../secret"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
}

func TestNewBuiltinExecutorGrepRejectsEmptyPattern(t *testing.T) {
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolGrep,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolGrep,
		Arguments: `{"pattern":"   "}`,
	})
	if !errors.Is(err, ErrToolPatternRequired) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPatternRequired)
	}
}
