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
