package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestNewBashHandlerWithTimeoutCancelsLongRunningCommand(t *testing.T) {
	handler := newBashHandlerWithTimeout(t.TempDir(), 20*time.Millisecond)

	start := time.Now()
	_, err := handler(runtime.ToolCall{
		Name:      ToolBash,
		Arguments: `{"command":"sleep 1"}`,
	}, Definition{Name: ToolBash, Kind: KindCommand})
	if !errors.Is(err, ErrToolCommandTimedOut) {
		t.Fatalf("handler() error = %v, want wrapped %v", err, ErrToolCommandTimedOut)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("handler() error = %v, want tool-level timeout error instead of raw context error", err)
	}
	if time.Since(start) >= 500*time.Millisecond {
		t.Fatalf("handler() duration = %v, want less than %v", time.Since(start), 500*time.Millisecond)
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

func TestNewBuiltinExecutorWriteFileCreatesParentDirectories(t *testing.T) {
	workDir := t.TempDir()
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolWriteFile,
			Kind: KindFile,
		},
	}, workDir)

	got, err := executor.Execute(runtime.ToolCall{
		Name:      ToolWriteFile,
		Arguments: `{"path":"notes/output.txt","content":"hello writer"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "wrote 12 bytes to notes/output.txt" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "wrote 12 bytes to notes/output.txt")
	}

	data, err := os.ReadFile(filepath.Join(workDir, "notes", "output.txt"))
	if err != nil {
		t.Fatalf("ReadFile(output.txt) error = %v", err)
	}
	if string(data) != "hello writer" {
		t.Fatalf("written content = %q, want %q", string(data), "hello writer")
	}
}

func TestNewBuiltinExecutorWriteFileOverwritesExistingFile(t *testing.T) {
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolWriteFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolWriteFile,
		Arguments: `{"path":"notes.txt","content":"new content"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile(notes.txt) error = %v", err)
	}
	if string(data) != "new content" {
		t.Fatalf("written content = %q, want %q", string(data), "new content")
	}
}

func TestNewBuiltinExecutorWriteFileRejectsPathOutsideWorkspace(t *testing.T) {
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolWriteFile,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolWriteFile,
		Arguments: `{"path":"../secret.txt","content":"x"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
}

func TestNewBuiltinExecutorReplaceFileReplacesSingleOccurrence(t *testing.T) {
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("hello old world"), 0o640); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, workDir)

	got, err := executor.Execute(runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"old","new":"new"}`,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "replaced 1 occurrence in notes.txt" {
		t.Fatalf("Execute().Output = %q, want %q", got.Output, "replaced 1 occurrence in notes.txt")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile(notes.txt) error = %v", err)
	}
	if string(data) != "hello new world" {
		t.Fatalf("replaced content = %q, want %q", string(data), "hello new world")
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Stat(notes.txt) error = %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("notes.txt mode = %#o, want %#o", info.Mode().Perm(), 0o640)
	}
}

func TestNewBuiltinExecutorReplaceFileReturnsErrorWhenTargetMissing(t *testing.T) {
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"missing","new":"new"}`,
	})
	if !errors.Is(err, ErrToolReplaceTargetMissing) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolReplaceTargetMissing)
	}
}

func TestNewBuiltinExecutorReplaceFileReturnsErrorWhenTargetNotUnique(t *testing.T) {
	workDir := t.TempDir()
	targetPath := filepath.Join(workDir, "notes.txt")
	if err := os.WriteFile(targetPath, []byte("old and old"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, workDir)

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"old","new":"new"}`,
	})
	if !errors.Is(err, ErrToolReplaceTargetNotUnique) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolReplaceTargetNotUnique)
	}
}

func TestNewBuiltinExecutorReplaceFileRejectsEmptyOldText(t *testing.T) {
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"notes.txt","old":"","new":"new"}`,
	})
	if !errors.Is(err, ErrToolReplaceOldRequired) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolReplaceOldRequired)
	}
}

func TestNewBuiltinExecutorReplaceFileRejectsPathOutsideWorkspace(t *testing.T) {
	executor := NewBuiltinExecutor([]Definition{
		{
			Name: ToolReplaceFile,
			Kind: KindFile,
		},
	}, t.TempDir())

	_, err := executor.Execute(runtime.ToolCall{
		Name:      ToolReplaceFile,
		Arguments: `{"path":"../secret.txt","old":"x","new":"y"}`,
	})
	if !errors.Is(err, ErrToolPathOutsideWorkspace) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ErrToolPathOutsideWorkspace)
	}
}
