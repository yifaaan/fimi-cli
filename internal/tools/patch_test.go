package tools

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"fimi-cli/internal/runtime"
)

func TestPatchFile_SimplePatch(t *testing.T) {
	// 创建临时工作目录
	workDir := t.TempDir()
	testFile := filepath.Join(workDir, "test.txt")
	err := os.WriteFile(testFile, []byte("Line 1\nLine 2\nLine 3\n"), 0o644)
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// 构造 simple patch
	patch := `--- test.txt	2024-01-01 00:00:00.000000000 +0000
+++ test.txt	2024-01-01 00:00:00.000000000 +0000
@@ -1,3 +1,3 @@
 Line 1
-Line 2
+Modified Line 2
 Line 3
`

	handler := newPatchFileHandler(workDir)
	call := runtime.ToolCall{
		Name:      "patch_file",
		Arguments: `{"path": ` + jsonString(testFile) + `, "diff": ` + jsonString(patch) + `}`,
	}

	result, err := handler(context.Background(), call, Definition{})
	if err != nil {
		t.Fatalf("patch_file failed: %v", err)
	}

	// 验证输出
	if result.Output == "" {
		t.Error("expected output message")
	}
	if got, want := result.Output, "Edited test.txt (+1 -1)"; got != want {
		t.Fatalf("result.Output = %q, want %q", got, want)
	}
	if result.DisplayOutput == "" {
		t.Fatal("result.DisplayOutput = empty, want diff preview")
	}
	for _, fragment := range []string{
		"Edited test.txt (+1 -1)",
		"@@ -1,3 +1,3 @@",
		"-Line 2",
		"+Modified Line 2",
	} {
		if !strings.Contains(result.DisplayOutput, fragment) {
			t.Fatalf("result.DisplayOutput = %q, want fragment %q", result.DisplayOutput, fragment)
		}
	}

	// 验证文件内容
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	expected := "Line 1\nModified Line 2\nLine 3\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

func TestPatchFile_MultipleHunks(t *testing.T) {
	workDir := t.TempDir()
	testFile := filepath.Join(workDir, "test.txt")
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
	err := os.WriteFile(testFile, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	patch := `--- test.txt	2024-01-01 00:00:00.000000000 +0000
+++ test.txt	2024-01-01 00:00:00.000000000 +0000
@@ -1,3 +1,3 @@
 Line 1
-Line 2
+Modified Line 2
 Line 3
@@ -4,2 +4,2 @@
 Line 4
-Line 5
+Modified Line 5
`

	handler := newPatchFileHandler(workDir)
	call := runtime.ToolCall{
		Name:      "patch_file",
		Arguments: `{"path": ` + jsonString(testFile) + `, "diff": ` + jsonString(patch) + `}`,
	}

	result, err := handler(context.Background(), call, Definition{})
	if err != nil {
		t.Fatalf("patch_file failed: %v", err)
	}
	if got, want := result.Output, "Edited test.txt (+2 -2)"; got != want {
		t.Fatalf("result.Output = %q, want %q", got, want)
	}

	// 验证文件内容
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	expected := "Line 1\nModified Line 2\nLine 3\nLine 4\nModified Line 5\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
	t.Log(result.Output)
}

func TestPatchFile_AddingLines(t *testing.T) {
	workDir := t.TempDir()
	testFile := filepath.Join(workDir, "test.txt")
	err := os.WriteFile(testFile, []byte("First line\nLast line\n"), 0o644)
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	patch := `--- test.txt	2023-01-01 00:00:00.000000000 +0000
+++ test.txt	2023-01-01 00:00:00.000000000 +0000
@@ -1,2 +1,3 @@
 First line
+New middle line
 Last line
`

	handler := newPatchFileHandler(workDir)
	call := runtime.ToolCall{
		Name:      "patch_file",
		Arguments: `{"path": ` + jsonString(testFile) + `, "diff": ` + jsonString(patch) + `}`,
	}

	_, err = handler(context.Background(), call, Definition{})
	if err != nil {
		t.Fatalf("patch_file failed: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	expected := "First line\nNew middle line\nLast line\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}

func TestPatchFile_RemovingLines(t *testing.T) {
	workDir := t.TempDir()
	testFile := filepath.Join(workDir, "test.txt")
	content := "First line\nMiddle line to remove\nLast line\n"
	err := os.WriteFile(testFile, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	patch := `--- test.txt	2023-01-01 00:00:00.000000000 +0000
+++ test.txt	2023-01-01 00:00:00.000000000 +0000
@@ -1,3 +1,2 @@
 First line
-Middle line to remove
 Last line
`

	handler := newPatchFileHandler(workDir)
	call := runtime.ToolCall{
		Name:      "patch_file",
		Arguments: `{"path": ` + jsonString(testFile) + `, "diff": ` + jsonString(patch) + `}`,
	}

	_, err = handler(context.Background(), call, Definition{})
	if err != nil {
		t.Fatalf("patch_file failed: %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	expected := "First line\nLast line\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}

func TestPatchFile_ApplyPatchStyleUpdateFile(t *testing.T) {
	workDir := t.TempDir()
	testFile := filepath.Join(workDir, "a.cpp")
	content := "#include <iostream>\n\nusing namesapce std;\n\nint main() {\n    return 0;\n}\n"
	err := os.WriteFile(testFile, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	patch := `*** Begin Patch
*** Update File: a.cpp
@@
-using namesapce std;
+using namespace std;
+
+void print_value(int value) {
+    std::cout << value << std::endl;
+}
+
+int max_value(int a, int b) {
+    return a > b ? a : b;
+}
*** End Patch
`

	handler := newPatchFileHandler(workDir)
	call := runtime.ToolCall{
		Name:      "patch_file",
		Arguments: `{"path": ` + jsonString("a.cpp") + `, "diff": ` + jsonString(patch) + `}`,
	}

	result, err := handler(context.Background(), call, Definition{})
	if err != nil {
		t.Fatalf("patch_file failed: %v", err)
	}
	if got, want := result.Output, "Edited a.cpp (+9 -1)"; got != want {
		t.Fatalf("result.Output = %q, want %q", got, want)
	}
	if !strings.Contains(result.DisplayOutput, "@@ -3,1 +3,9 @@") {
		t.Fatalf("result.DisplayOutput = %q, want resolved unified hunk header", result.DisplayOutput)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	got := string(data)
	for _, fragment := range []string{
		"using namespace std;",
		"void print_value(int value) {",
		"int max_value(int a, int b) {",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("patched file = %q, want fragment %q", got, fragment)
		}
	}
}

func TestPatchFile_ApplyPatchStyleDisplayOutputUsesResolvedLineNumbers(t *testing.T) {
	workDir := t.TempDir()
	testFile := filepath.Join(workDir, "a.cpp")
	lines := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	if err := os.WriteFile(testFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	patch := `*** Begin Patch
*** Update File: a.cpp
@@
 line 72
 line 73
+bool isSorted(const std::vector<int>& arr) {
+    return true;
+}
 line 74
 line 75
*** End Patch
`

	handler := newPatchFileHandler(workDir)
	result, err := handler(context.Background(), runtime.ToolCall{
		Name:      "patch_file",
		Arguments: `{"path": ` + jsonString("a.cpp") + `, "diff": ` + jsonString(patch) + `}`,
	}, Definition{})
	if err != nil {
		t.Fatalf("patch_file failed: %v", err)
	}

	if !strings.Contains(result.DisplayOutput, "@@ -72,4 +72,7 @@") {
		t.Fatalf("result.DisplayOutput = %q, want resolved hunk lines near actual file location", result.DisplayOutput)
	}
}

func TestApplyUnifiedDiff_ApplyPatchStyleWithoutLineNumbers(t *testing.T) {
	original := "alpha\nbeta\ngamma\n"
	patch := `*** Begin Patch
*** Update File: test.txt
@@
 alpha
-beta
+beta updated
 gamma
*** End Patch
`

	got, hunksApplied, err := applyUnifiedDiff(original, patch)
	if err != nil {
		t.Fatalf("applyUnifiedDiff() error = %v", err)
	}
	if hunksApplied != 1 {
		t.Fatalf("hunksApplied = %d, want 1", hunksApplied)
	}
	if got != "alpha\nbeta updated\ngamma\n" {
		t.Fatalf("patched content = %q, want %q", got, "alpha\nbeta updated\ngamma\n")
	}
}

func TestPatchFile_ErrorCases(t *testing.T) {
	workDir := t.TempDir()

	handler := newPatchFileHandler(workDir)

	tests := []struct {
		name        string
		path        string
		diff        string
		setupFile   bool
		fileContent string
	}{
		{
			name:      "relative path error",
			path:      "relative/path/file.txt",
			diff:      "--- test\n+++ test\n@@ -1 +1 @@\n-old\n+new\n",
			setupFile: false,
		},
		{
			name:      "nonexistent file",
			path:      filepath.Join(workDir, "nonexistent.txt"),
			diff:      "--- test\n+++ test\n@@ -1 +1 @@\n-old\n+new\n",
			setupFile: false,
		},
		{
			name:        "invalid diff format",
			path:        filepath.Join(workDir, "test.txt"),
			diff:        "This is not a valid diff",
			setupFile:   true,
			fileContent: "content",
		},
		{
			name:        "mismatched patch",
			path:        filepath.Join(workDir, "test.txt"),
			diff:        "--- test.txt\n+++ test.txt\n@@ -1,3 +1,3 @@\nFirst line\n-Second line\n+Modified second line\nThird line\n",
			setupFile:   true,
			fileContent: "Different content",
		},
		{
			name:        "no changes made",
			path:        filepath.Join(workDir, "test.txt"),
			diff:        "--- test.txt\n+++ test.txt\n@@ -1,2 +1,2 @@\nLine 1\n Line 2\n",
			setupFile:   true,
			fileContent: "Line 1\nLine 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFile {
				err := os.WriteFile(tt.path, []byte(tt.fileContent), 0o644)
				if err != nil {
					t.Fatalf("setup file: %v", err)
				}
			}

			call := runtime.ToolCall{
				Name:      "patch_file",
				Arguments: `{"path": ` + jsonString(tt.path) + `, "diff": ` + jsonString(tt.diff) + `}`,
			}

			_, err := handler(context.Background(), call, Definition{})
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestApplyUnifiedDiff_InvalidHunkHeader(t *testing.T) {
	_, _, err := applyUnifiedDiff("content", "invalid diff without hunk header")
	if err == nil {
		t.Error("expected error for invalid diff")
	}
}

func TestApplyUnifiedDiff_EmptyDiff(t *testing.T) {
	_, _, err := applyUnifiedDiff("content", "")
	if err == nil {
		t.Error("expected error for empty diff")
	}
}

// jsonString 返回一个 JSON 字符串的字面量表示
func jsonString(s string) string {
	// 简单的 JSON 字符串转义
	result := `"`
	for _, c := range s {
		switch c {
		case '"':
			result += `\"`
		case '\\':
			result += `\\`
		case '\n':
			result += `\n`
		case '\t':
			result += `\t`
		case '\r':
			result += `\r`
		default:
			result += string(c)
		}
	}
	result += `"`
	return result
}
