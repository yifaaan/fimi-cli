# Toast Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a stackable, auto-dismissing toast notification system to the Bubble Tea shell UI, fed by runtime events via the Wire channel.

**Architecture:** New `ToastModel` sub-model in `internal/ui/shell/toast.go`, parallel to existing InputModel/OutputModel/RuntimeModel. Runtime sends toasts through a new `EventToast` event type that flows through the existing `wire.EventMessage` channel. Toasts render as inline rows above the input area.

**Tech Stack:** Go, Bubble Tea, lipgloss

**Spec:** `docs/superpowers/specs/2026-03-31-toast-notifications-design.md`

---

### Task 1: Add EventToast event type

**Files:**
- Modify: `internal/runtime/events/events.go` (after line 15, before line 103)

- [ ] **Step 1: Add KindToast constant and EventToast struct**

In `internal/runtime/events/events.go`, add a new Kind constant at line 16 (after `KindToolResult`) and a new event struct (after the `ToolResult` struct around line 103):

```go
// Add to Kind const block after KindToolResult (line 15):
KindToast Kind = "toast"

// Add after ToolResult struct (after line 103):
type EventToast struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Action  string `json:"action,omitempty"`
}

func (e EventToast) Kind() Kind { return KindToast }
```

- [ ] **Step 2: Run existing tests to verify no breakage**

Run: `go build ./internal/runtime/...`
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add internal/runtime/events/events.go
git commit -m "feat(events): add EventToast event type for toast notifications"
```

---

### Task 2: Add toast level styles

**Files:**
- Modify: `internal/ui/shell/styles/lipgloss.go` (after line 111)

- [ ] **Step 1: Add toast styles**

Append to `internal/ui/shell/styles/lipgloss.go`:

```go
// Toast styles
var (
	ToastInfoStyle = lipgloss.NewStyle().
			Foreground(ColorInfo).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorInfo).
			Padding(0, 1)

	ToastWarningStyle = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				Padding(0, 1)

	ToastErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorError).
			Padding(0, 1)

	ToastSuccessStyle = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSuccess).
				Padding(0, 1)
)
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/ui/shell/...`
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/shell/styles/lipgloss.go
git commit -m "feat(styles): add toast level lipgloss styles"
```

---

### Task 3: Create ToastModel

**Files:**
- Create: `internal/ui/shell/toast.go`

- [ ] **Step 1: Write ToastModel with types, Update, and View**

Create `internal/ui/shell/toast.go`:

```go
package shell

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fimi-cli/internal/ui/shell/styles"
)

// ToastLevel represents the severity/type of a toast notification.
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastWarning
	ToastError
	ToastSuccess
)

// Toast is a single notification item.
type Toast struct {
	ID        int64
	Level     ToastLevel
	Message   string
	Detail    string
	Action    string
	CreatedAt time.Time
	TTL       time.Duration
}

// Bubble Tea messages.
type ToastAddMsg struct{ Toast Toast }
type ToastDismissMsg struct{ ID int64 }
type ToastTickMsg struct{ Time time.Time }

// ToastModel manages a stack of transient toast notifications.
type ToastModel struct {
	toasts   []Toast
	maxStack int
	nextID   int64
	width    int
}

func NewToastModel() ToastModel {
	return ToastModel{
		maxStack: 5,
	}
}

func (m ToastModel) SetWidth(w int) ToastModel {
	m.width = w
	return m
}

func (m ToastModel) Update(msg tea.Msg) (ToastModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ToastAddMsg:
		t := msg.Toast
		t.ID = m.nextID
		m.nextID++
		if t.CreatedAt.IsZero() {
			t.CreatedAt = time.Now()
		}
		if t.TTL == 0 {
			t.TTL = 5 * time.Second
		}
		m.toasts = append(m.toasts, t)
		if len(m.toasts) > m.maxStack {
			m.toasts = m.toasts[len(m.toasts)-m.maxStack:]
		}
		return m, scheduleToastTick()

	case ToastDismissMsg:
		for i, t := range m.toasts {
			if t.ID == msg.ID {
				m.toasts = append(m.toasts[:i], m.toasts[i+1:]...)
				break
			}
		}
		if len(m.toasts) > 0 {
			return m, scheduleToastTick()
		}
		return m, nil

	case ToastTickMsg:
		now := time.Now()
		remaining := m.toasts[:0]
		for _, t := range m.toasts {
			if now.Sub(t.CreatedAt) < t.TTL {
				remaining = append(remaining, t)
			}
		}
		m.toasts = remaining
		if len(m.toasts) > 0 {
			return m, scheduleToastTick()
		}
		return m, nil
	}
	return m, nil
}

func (m ToastModel) View() string {
	if len(m.toasts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range m.toasts {
		style := toastStyle(t.Level)
		icon := toastIcon(t.Level)
		line := fmt.Sprintf(" %s %s", icon, t.Message)
		if t.Detail != "" {
			line += "  " + t.Detail
		}
		if t.Action != "" {
			line += fmt.Sprintf("  [%s]", t.Action)
		}
		if m.width > 0 {
			b.WriteString(style.Width(m.width).Render(line))
		} else {
			b.WriteString(style.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func scheduleToastTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return ToastTickMsg{Time: t}
	})
}

func toastStyle(level ToastLevel) lipgloss.Style {
	switch level {
	case ToastInfo:
		return styles.ToastInfoStyle
	case ToastWarning:
		return styles.ToastWarningStyle
	case ToastError:
		return styles.ToastErrorStyle
	case ToastSuccess:
		return styles.ToastSuccessStyle
	default:
		return styles.ToastInfoStyle
	}
}

func toastIcon(level ToastLevel) string {
	switch level {
	case ToastInfo:
		return "ℹ"
	case ToastWarning:
		return "⚠"
	case ToastError:
		return "✗"
	case ToastSuccess:
		return "✓"
	default:
		return "ℹ"
	}
}

func parseToastLevel(s string) ToastLevel {
	switch s {
	case "warning":
		return ToastWarning
	case "error":
		return ToastError
	case "success":
		return ToastSuccess
	default:
		return ToastInfo
	}
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/ui/shell/...`
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/shell/toast.go
git commit -m "feat(shell): add ToastModel sub-model for toast notifications"
```

---

### Task 4: Write ToastModel tests

**Files:**
- Create: `internal/ui/shell/toast_test.go`

- [ ] **Step 1: Write tests for ToastModel**

Create `internal/ui/shell/toast_test.go`:

```go
package shell

import (
	"testing"
	"time"
)

func TestToastAdd(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{
		Level:   ToastInfo,
		Message: "hello",
	}})

	if len(m.toasts) != 1 {
		t.Fatalf("expected 1 toast, got %d", len(m.toasts))
	}
	if m.toasts[0].Message != "hello" {
		t.Errorf("expected message 'hello', got %q", m.toasts[0].Message)
	}
	if m.toasts[0].ID != 0 {
		t.Errorf("expected ID 0, got %d", m.toasts[0].ID)
	}
	if m.toasts[0].TTL != 5*time.Second {
		t.Errorf("expected default TTL 5s, got %v", m.toasts[0].TTL)
	}
	if m.toasts[0].CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestToastStackLimit(t *testing.T) {
	m := NewToastModel()
	for i := 0; i < 7; i++ {
		m, _ = m.Update(ToastAddMsg{Toast: Toast{
			Level:   ToastInfo,
			Message: "msg",
		}})
	}

	if len(m.toasts) != 5 {
		t.Fatalf("expected 5 toasts (max stack), got %d", len(m.toasts))
	}
	// oldest should be dropped, so IDs start at 2
	if m.toasts[0].ID != 2 {
		t.Errorf("expected first toast ID 2, got %d", m.toasts[0].ID)
	}
}

func TestToastDismiss(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{Level: ToastInfo, Message: "first"}})
	m, _ = m.Update(ToastAddMsg{Toast: Toast{Level: ToastInfo, Message: "second"}})

	m, _ = m.Update(ToastDismissMsg{ID: 0})
	if len(m.toasts) != 1 {
		t.Fatalf("expected 1 toast after dismiss, got %d", len(m.toasts))
	}
	if m.toasts[0].Message != "second" {
		t.Errorf("expected 'second' remaining, got %q", m.toasts[0].Message)
	}
}

func TestToastTickExpires(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{
		Level:     ToastInfo,
		Message:   "expires",
		CreatedAt: time.Now().Add(-6 * time.Second), // already expired
		TTL:       5 * time.Second,
	}})

	m, _ = m.Update(ToastTickMsg{Time: time.Now()})
	if len(m.toasts) != 0 {
		t.Fatalf("expected 0 toasts after expiry, got %d", len(m.toasts))
	}
}

func TestToastTickKeepsAlive(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{
		Level:     ToastInfo,
		Message:   "fresh",
		CreatedAt: time.Now(),
		TTL:       5 * time.Second,
	}})

	m, _ = m.Update(ToastTickMsg{Time: time.Now()})
	if len(m.toasts) != 1 {
		t.Fatalf("expected 1 toast still alive, got %d", len(m.toasts))
	}
}

func TestToastViewEmpty(t *testing.T) {
	m := NewToastModel()
	if v := m.View(); v != "" {
		t.Errorf("expected empty view, got %q", v)
	}
}

func TestToastViewNonEmpty(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{Level: ToastSuccess, Message: "done"}})
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view")
	}
	if !contains(v, "done") {
		t.Errorf("expected view to contain 'done', got %q", v)
	}
}

func TestParseToastLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected ToastLevel
	}{
		{"info", ToastInfo},
		{"warning", ToastWarning},
		{"error", ToastError},
		{"success", ToastSuccess},
		{"unknown", ToastInfo},
		{"", ToastInfo},
	}
	for _, tt := range tests {
		got := parseToastLevel(tt.input)
		if got != tt.expected {
			t.Errorf("parseToastLevel(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && findSub(s, sub)))
}

func findSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/ui/shell/ -run TestToast -v`
Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/shell/toast_test.go
git commit -m "test(shell): add ToastModel unit tests"
```

---

### Task 5: Wire EventToast into wireReceiveLoop

**Files:**
- Modify: `internal/ui/shell/model_runtime.go` (function `eventToTeaMsg`, lines 234-253)

- [ ] **Step 1: Add EventToast case to eventToTeaMsg**

In `model_runtime.go`, add a case to the `eventToTeaMsg` function's type switch (after the last event case, before `default`):

```go
case runtimeevents.EventToast:
	return ToastAddMsg{Toast: Toast{
		Level:   parseToastLevel(e.Level),
		Message: e.Message,
		Detail:  e.Detail,
		Action:  e.Action,
	}}
```

Note: `CreatedAt` and `TTL` are left zero — `ToastModel.Update` will set defaults.

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/ui/shell/...`
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/shell/model_runtime.go
git commit -m "feat(shell): convert EventToast wire events to ToastAddMsg"
```

---

### Task 6: Integrate ToastModel into main Model

**Files:**
- Modify: `internal/ui/shell/model.go`

- [ ] **Step 1: Add toasts field to Model struct**

In the Model struct (after line 56, `runtime RuntimeModel`), add:

```go
toasts  ToastModel
```

- [ ] **Step 2: Add toast message cases to Update() switch**

In `Update()` method (around line 291, after the `FileIndexResultMsg` case), add:

```go
case ToastAddMsg, ToastDismissMsg, ToastTickMsg:
	var cmd tea.Cmd
	m.toasts, cmd = m.toasts.Update(msg)
	return m, cmd
```

- [ ] **Step 3: Insert toast View() into the View() rendering order**

In `View()` method, find where the input area is appended (around line 339):

```go
sections = append(sections, m.input.View())
```

Insert **before** this line:

```go
sections = append(sections, m.toasts.View())
```

The order becomes: output → live status → toasts → input.

- [ ] **Step 4: Propagate width to ToastModel on resize**

In the `tea.WindowSizeMsg` case in `Update()` (around line 228), add after the existing sub-model width updates:

```go
m.toasts = m.toasts.SetWidth(msg.Width)
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/ui/shell/...`
Expected: compiles without errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/shell/model.go
git commit -m "feat(shell): integrate ToastModel into main shell Model"
```

---

### Task 7: Build verification

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: compiles without errors.

- [ ] **Step 2: Run all shell tests**

Run: `go test ./internal/ui/shell/... -v`
Expected: all tests pass.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: all tests pass (no regressions).
