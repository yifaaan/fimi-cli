# Shell Native Mouse Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore terminal-native mouse wheel scrolling and drag text selection by removing shell-level mouse capture.

**Architecture:** Remove Bubble Tea mouse capture from shell startup so the hosting terminal owns wheel scrolling and text selection again. Keep shell interaction keyboard-driven, and update help text plus focused shell tests to match the new mouse contract.

**Tech Stack:** Go, Bubble Tea, shell UI tests.

---

## File Structure

| File | Purpose |
|------|---------|
| `internal/ui/shell/shell.go` | Remove unconditional mouse capture from Bubble Tea startup and update help text to describe keyboard-only shell controls. |
| `internal/ui/shell/shell_test.go` | Add focused regression coverage for updated help text and existing resume command behavior. |

---

### Task 1: Update help text to stop promising shell-owned mouse behavior

**Files:**
- Modify: `internal/ui/shell/shell.go:18-45`
- Modify: `internal/ui/shell/shell_test.go`
- Test: `internal/ui/shell/shell_test.go`

- [ ] **Step 1: Write the failing help-text regression test**

Add this test to `internal/ui/shell/shell_test.go` after `TestResumeCommandText`:

```go
func TestHelpTextDescribesKeyboardShortcutsWithoutMouseWheelLine(t *testing.T) {
	got := helpText()

	for _, want := range []string{
		"Available commands:",
		"Keyboard shortcuts:",
		"Ctrl+O          Toggle tool result expansion",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("helpText() missing %q in:\n%s", want, got)
		}
	}

	for _, unwanted := range []string{
		"Mouse wheel     Scroll transcript history",
		"mouse wheel",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("helpText() unexpectedly contains %q in:\n%s", unwanted, got)
		}
	}
}
```

Also update the imports at the top of `internal/ui/shell/shell_test.go` to:

```go
import (
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/ui/shell -run TestHelpTextDescribesKeyboardShortcutsWithoutMouseWheelLine -count=1
```

Expected: FAIL because `helpText()` still includes `Mouse wheel     Scroll transcript history`.

- [ ] **Step 3: Update the help text**

In `internal/ui/shell/shell.go`, replace `helpText()` with:

```go
func helpText() string {
	lines := []string{
		"Available commands:",
		"  /help           Show this help message",
		"  /clear          Clear the screen",
		"  /compact        Compact conversation context",
		"  /init           Generate AGENTS.md for the project",
		"  /rewind         List available rewind checkpoints",
		"  /version        Show version information",
		"  /release-notes  Show release notes",
		"  /exit, /quit    Exit the shell",
		"  /resume         List available sessions",
		"  /resume <id>    Switch to a specific session",
		"  /task           List background tasks",
		"  /task <id>      Show background task status",
		"  /task kill <id> Kill a background task",
		"  /setup          Setup LLM provider and model",
		"  /reload         Reload configuration",
		"",
		"Keyboard shortcuts:",
		"  Ctrl+C/Ctrl+D   Exit (when idle)",
		"  Ctrl+L          Clear screen",
		"  Ctrl+O          Toggle tool result expansion",
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run the help-text test to verify it passes**

Run:

```bash
go test ./internal/ui/shell -run 'TestResumeCommandText|TestHelpTextDescribesKeyboardShortcutsWithoutMouseWheelLine' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the help-text change**

Run:

```bash
git add internal/ui/shell/shell.go internal/ui/shell/shell_test.go
git commit -m "fix(shell): describe native mouse behavior"
```

Expected: commit succeeds with updated shell help text and regression coverage.

---

### Task 2: Remove Bubble Tea mouse capture from shell startup

**Files:**
- Modify: `internal/ui/shell/shell.go:163-170`
- Test: `internal/ui/shell/shell_test.go`

- [ ] **Step 1: Write the startup regression test**

Add this test to `internal/ui/shell/shell_test.go` after `TestHelpTextDescribesKeyboardShortcutsWithoutMouseWheelLine`:

```go
func TestShellSourceDoesNotEnableMouseCapture(t *testing.T) {
	data, err := os.ReadFile("shell.go")
	if err != nil {
		t.Fatalf("os.ReadFile(shell.go) error = %v", err)
	}

	source := string(data)
	if strings.Contains(source, "tea.WithMouseCellMotion()") {
		t.Fatalf("shell.go unexpectedly enables mouse capture:\n%s", source)
	}
	if !strings.Contains(source, "tea.NewProgram(") {
		t.Fatalf("shell.go missing tea.NewProgram setup:\n%s", source)
	}
}
```

Then update the imports in `internal/ui/shell/shell_test.go` to:

```go
import (
	"os"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/ui/shell -run TestShellSourceDoesNotEnableMouseCapture -count=1
```

Expected: FAIL because `shell.go` still contains `tea.WithMouseCellMotion()`.

- [ ] **Step 3: Remove mouse capture from shell startup**

In `internal/ui/shell/shell.go`, replace the `tea.NewProgram` block:

```go
	p := tea.NewProgram(
		model,
		tea.WithInput(input),
		tea.WithOutput(output),
		tea.WithMouseCellMotion(),
	)
```

with:

```go
	p := tea.NewProgram(
		model,
		tea.WithInput(input),
		tea.WithOutput(output),
	)
```

Keep the surrounding comment, but update it to match the actual behavior if needed so it clearly says the shell does not capture the mouse and relies on terminal-native selection/scrolling.

- [ ] **Step 4: Run the focused shell tests to verify they pass**

Run:

```bash
go test ./internal/ui/shell -run 'TestResumeCommandText|TestHelpTextDescribesKeyboardShortcutsWithoutMouseWheelLine|TestShellSourceDoesNotEnableMouseCapture' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the mouse-capture removal**

Run:

```bash
git add internal/ui/shell/shell.go internal/ui/shell/shell_test.go
git commit -m "fix(shell): restore native mouse selection"
```

Expected: commit succeeds with mouse-capture removal and focused regression coverage.

---

### Task 3: Verify full shell package stability

**Files:**
- Test: `internal/ui/shell/...`

- [ ] **Step 1: Run the full shell package test suite**

Run:

```bash
go test ./internal/ui/shell/... -count=1
```

Expected: PASS for the full shell package with no new failures.

- [ ] **Step 2: Record manual verification steps**

Use this exact checklist for manual validation in Windows Terminal:

```text
1. Launch the shell in Windows Terminal.
2. Rotate the mouse wheel and confirm Windows Terminal scrollback moves.
3. Drag-select transcript text and confirm the text is highlighted normally.
4. Press Ctrl+O and confirm preview expansion still works.
5. Submit a prompt and confirm input/editing still works.
```

Expected: all five checks succeed.

- [ ] **Step 3: Commit only if Task 1 and Task 2 were intentionally squashed earlier**

If Task 1 and Task 2 were already committed separately, do not create an extra commit here.

If you intentionally performed the work without intermediate commits, create a single final commit with:

```bash
git add internal/ui/shell/shell.go internal/ui/shell/shell_test.go
git commit -m "fix(shell): use terminal-native mouse behavior"
```

Expected: no extra commit if the earlier task commits already exist; otherwise one final commit exists.

---

## Spec Coverage Check

- **Remove app-level mouse capture:** Task 2 removes `tea.WithMouseCellMotion()` from shell startup.
- **Stop promising shell-owned wheel scrolling:** Task 1 updates help text so it no longer advertises mouse-wheel transcript scrolling.
- **Keep keyboard interactions unchanged:** Tasks only touch shell startup and help text; no keyboard handlers or transcript rendering logic are changed.
- **Run shell test coverage:** Task 3 runs the full shell package suite and records the manual Windows Terminal checks.

## Placeholder Scan

- No `TODO`, `TBD`, or deferred steps remain.
- Every code change includes exact code.
- Every test step includes exact commands and expected results.
- File paths are explicit throughout.

## Type Consistency Check

- `helpText()` remains in `internal/ui/shell/shell.go` and is referenced consistently by the new tests.
- `tea.NewProgram(...)` remains the startup point in `internal/ui/shell/shell.go`.
- `TestResumeCommandText`, `TestHelpTextDescribesKeyboardShortcutsWithoutMouseWheelLine`, and `TestShellSourceDoesNotEnableMouseCapture` all live in `internal/ui/shell/shell_test.go` and use standard library imports only.
