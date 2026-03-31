# Shell Parity — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `/version`, `/release-notes`, toolbar keyboard hints, and `/setup` interactive config wizard for shell parity with Python reference.

**Architecture:** Add changelog package for embedded release notes, extend shell model with setup mode and phase handlers, add config.Save() for atomic writes.

**Tech Stack:** Go, Bubble Tea (charmbracelet/bubbletea), lipgloss, go:embed directive.

---

## File Structure

| File | Purpose |
|------|---------|
| `internal/changelog/CHANGELOG.md` | Copy of changelog for embedding |
| `internal/changelog/changelog.go` | `//go:embed` directive + exported `Content` string |
| `internal/changelog/parser.go` | `ReleaseEntry` struct + `ParseReleases()` function |
| `internal/changelog/parser_test.go` | Parser unit tests |
| `internal/config/config.go` | Add `Save()` and `SaveFile()` functions |
| `internal/config/config_test.go` | Add `SaveFile` tests |
| `internal/ui/shell/model.go` | Add `ModeSetup`, `SetupState`, setup phase constants, `enterSetupMode()`, `handleSetupKeyPress()`, `renderSetupView()`, extend `renderStatusBar()`, add `/version`, `/release-notes`, `/setup` to `handleCommand()` |
| `internal/ui/shell/model_output.go` | Add `hasExpandedResults()` method |
| `internal/ui/shell/shell.go` | Add `versionText()`, `releaseNotesText()`, update `helpText()`, update `availableCommands()` |

---

## Task 1: `/version` Command

**Files:**
- Modify: `internal/ui/shell/model.go`
- Modify: `internal/ui/shell/shell.go`

- [ ] **Step 1: Add version case to handleCommand**

In `internal/ui/shell/model.go`, find `handleCommand()` switch block (around line 653). Add case before `default`:

```go
case cmd == "/version":
    m.output = m.output.AppendLine(TranscriptLine{
        Type:    LineTypeSystem,
        Content: versionText(m.deps.StartupInfo.AppVersion),
    })
    return m, nil
```

- [ ] **Step 2: Add versionText function in shell.go**

In `internal/ui/shell/shell.go`, add after `formatTime()` function (around line 52):

```go
// versionText returns the version display string.
func versionText(version string) string {
    version = strings.TrimSpace(version)
    if version == "" || version == "dev" {
        return "fimi-cli dev build"
    }
    return "fimi-cli v" + strings.TrimPrefix(version, "v")
}
```

- [ ] **Step 3: Add /version to availableCommands**

In `internal/ui/shell/model.go`, find `availableCommands()` function (around line 97). Add entry:

```go
{Name: "/version", Description: "Show version information"},
```

- [ ] **Step 4: Add /version to helpText**

In `internal/ui/shell/shell.go`, find `helpText()` function (around line 17). Add line after `/rewind`:

```go
"  /version        Show version information",
```

- [ ] **Step 5: Verify with manual test**

Build and run:
```bash
go build -o fimi ./cmd/fimi && ./fimi
```

In shell, type `/version`. Expected output: `fimi-cli dev build` (or version if built with tag).

- [ ] **Step 6: Commit**

```bash
git add internal/ui/shell/model.go internal/ui/shell/shell.go
git commit -m "feat(shell): add /version command

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 2: `/release-notes` Command

**Files:**
- Create: `internal/changelog/CHANGELOG.md`
- Create: `internal/changelog/changelog.go`
- Create: `internal/changelog/parser.go`
- Create: `internal/changelog/parser_test.go`
- Modify: `internal/ui/shell/model.go`
- Modify: `internal/ui/shell/shell.go`

- [ ] **Step 1: Copy CHANGELOG.md for embedding**

```bash
cp temp/CHANGELOG.md internal/changelog/CHANGELOG.md
```

- [ ] **Step 2: Create changelog.go with embedded content**

Create `internal/changelog/changelog.go`:

```go
package changelog

import _ "embed"

// Content is the embedded changelog markdown content.
//
//go:embed CHANGELOG.md
var Content string
```

- [ ] **Step 3: Write failing parser test**

Create `internal/changelog/parser_test.go`:

```go
package changelog

import (
	"testing"
)

func TestParseReleasesExtractsVersionAndDate(t *testing.T) {
	content := `## [0.35] - 2025-10-22

### Changed

- Minor UI improvements
- Auto download ripgrep if not found in the system

## [0.34] - 2025-10-21

### Added

- Add /update meta command
`

	entries := ParseReleases(content, 10)
	if len(entries) != 2 {
		t.Fatalf("ParseReleases() returned %d entries, want 2", len(entries))
	}

	if entries[0].Version != "0.35" {
		t.Fatalf("entries[0].Version = %q, want %q", entries[0].Version, "0.35")
	}
	if entries[0].Date != "2025-10-22" {
		t.Fatalf("entries[0].Date = %q, want %q", entries[0].Date, "2025-10-22")
	}
	if entries[1].Version != "0.34" {
		t.Fatalf("entries[1].Version = %q, want %q", entries[1].Version, "0.34")
	}
}

func TestParseReleasesCollectsBulletsAcrossSections(t *testing.T) {
	content := `## [0.35] - 2025-10-22

### Changed

- Minor UI improvements
- Auto download ripgrep

### Fixed

- Fix logging redirection

## [0.34] - 2025-10-21

### Added

- Add /update meta command
`

	entries := ParseReleases(content, 10)
	if len(entries) != 2 {
		t.Fatalf("ParseReleases() returned %d entries, want 2", len(entries))
	}

	// First entry should have 3 bullets (2 from Changed, 1 from Fixed)
	if len(entries[0].Bullets) != 3 {
		t.Fatalf("entries[0].Bullets len = %d, want 3", len(entries[0].Bullets))
	}
	if entries[0].Bullets[0] != "Minor UI improvements" {
		t.Fatalf("entries[0].Bullets[0] = %q, want %q", entries[0].Bullets[0], "Minor UI improvements")
	}
	if entries[0].Bullets[2] != "Fix logging redirection" {
		t.Fatalf("entries[0].Bullets[2] = %q, want %q", entries[0].Bullets[2], "Fix logging redirection")
	}
}

func TestParseReleasesRespectsLimit(t *testing.T) {
	content := `## [0.35] - 2025-10-22

### Changed

- Minor UI improvements

## [0.34] - 2025-10-21

### Added

- Add /update meta command

## [0.33] - 2025-10-18

### Fixed

- Fix logging
`

	entries := ParseReleases(content, 2)
	if len(entries) != 2 {
		t.Fatalf("ParseReleases() returned %d entries, want 2", len(entries))
	}
	if entries[0].Version != "0.35" {
		t.Fatalf("entries[0].Version = %q, want 0.35", entries[0].Version)
	}
	if entries[1].Version != "0.34" {
		t.Fatalf("entries[1].Version = %q, want 0.34", entries[1].Version)
	}
}

func TestParseReleasesReturnsEmptyForNoContent(t *testing.T) {
	entries := ParseReleases("", 10)
	if len(entries) != 0 {
		t.Fatalf("ParseReleases(\"\") returned %d entries, want 0", len(entries))
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test ./internal/changelog/... -v
```

Expected: compilation error or test failure (parser not implemented).

- [ ] **Step 5: Implement parser**

Create `internal/changelog/parser.go`:

```go
package changelog

import (
	"regexp"
	"strings"
)

// ReleaseEntry represents a parsed changelog version entry.
type ReleaseEntry struct {
	Version string
	Date    string
	Bullets []string
}

// ParseReleases parses changelog markdown and returns release entries.
// Returns entries in reverse chronological order (newest first), capped at limit.
func ParseReleases(content string, limit int) []ReleaseEntry {
	if content == "" || limit <= 0 {
		return nil
	}

	// Find all version headers: ## [version] - date
	headerPattern := regexp.MustCompile(`^## \[([^\]]+)\] - ([^\n]+)`)
	sections := strings.Split(content, "\n## [")

	var entries []ReleaseEntry
	for i, section := range sections {
		// First section may not start with ## [ (e.g. header comment)
		if i == 0 && !strings.HasPrefix(section, "[") {
			continue
		}

		// Normalize: add back ## [ prefix except for first section that already has it
		if i > 0 {
			section = "[" + section
		}

		lines := strings.Split(section, "\n")
		if len(lines) == 0 {
			continue
		}

		// Parse header line
		headerLine := lines[0]
		if i == 0 {
			headerLine = "## " + headerLine
		}
		matches := headerPattern.FindStringSubmatch(headerLine)
		if len(matches) < 3 {
			continue
		}

		entry := ReleaseEntry{
			Version: matches[1],
			Date:    strings.TrimSpace(matches[2]),
			Bullets: nil,
		}

		// Collect bullet lines across all subsections
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				bullet := strings.TrimPrefix(line, "- ")
				entry.Bullets = append(entry.Bullets, bullet)
			}
		}

		if len(entry.Bullets) > 0 {
			entries = append(entries, entry)
		}
	}

	// Entries are already in chronological order from file (oldest first)
	// Reverse to get newest first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	// Cap at limit
	if len(entries) > limit {
		entries = entries[:limit]
	}

	return entries
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test ./internal/changelog/... -v
```

Expected: all tests PASS.

- [ ] **Step 7: Add release-notes case to handleCommand**

In `internal/ui/shell/model.go`, find `handleCommand()` switch block. Add case after `/version`:

```go
case cmd == "/release-notes":
    entries := changelog.ParseReleases(changelog.Content, 5)
    m.output = m.output.AppendLine(TranscriptLine{
        Type:    LineTypeSystem,
        Content: releaseNotesText(entries),
    })
    return m, nil
```

Add import at top of file:
```go
"fimi-cli/internal/changelog"
```

- [ ] **Step 8: Add releaseNotesText function in shell.go**

In `internal/ui/shell/shell.go`, add after `versionText()`:

```go
// releaseNotesText formats parsed release entries for display.
func releaseNotesText(entries []changelog.ReleaseEntry) string {
	if len(entries) == 0 {
		return "No release notes available."
	}

	var lines []string
	lines = append(lines, "Release Notes:")
	lines = append(lines, "")

	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("## [%s] - %s", entry.Version, entry.Date))
		for _, bullet := range entry.Bullets {
			lines = append(lines, "  - "+bullet)
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
```

Add imports at top of `shell.go`:
```go
import (
    "fimi-cli/internal/changelog"
    // ... existing imports
)
```

- [ ] **Step 9: Add /release-notes to availableCommands**

In `internal/ui/shell/model.go`, `availableCommands()` function, add:

```go
{Name: "/release-notes", Description: "Show release notes"},
```

- [ ] **Step 10: Add /release-notes to helpText**

In `internal/ui/shell/shell.go`, `helpText()` function, add:

```go
"  /release-notes  Show release notes",
```

- [ ] **Step 11: Verify with manual test**

Build and run:
```bash
go build -o fimi ./cmd/fimi && ./fimi
```

In shell, type `/release-notes`. Expected: formatted changelog output with newest version first.

- [ ] **Step 12: Commit**

```bash
git add internal/changelog internal/ui/shell/model.go internal/ui/shell/shell.go
git commit -m "feat(shell): add /release-notes command with embedded changelog

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 3: Bottom Toolbar Keyboard Hint

**Files:**
- Modify: `internal/ui/shell/model_output.go`
- Modify: `internal/ui/shell/model.go`

- [ ] **Step 1: Add hasExpandedResults method to OutputModel**

In `internal/ui/shell/model_output.go`, add after `ToggleExpand()` function (around line 393):

```go
// HasExpandedResults returns true if any tool result is currently expanded.
func (m OutputModel) HasExpandedResults() bool {
	for _, v := range m.expanded {
		if v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Add currentShortcutHint method to Model**

In `internal/ui/shell/model.go`, add after `renderStatusBar()` function (around line 1538):

```go
// currentShortcutHint returns a context-appropriate keyboard hint.
func (m Model) currentShortcutHint() string {
	switch m.mode {
	case ModeStreaming, ModeThinking:
		return ""
	}
	if m.output.HasExpandedResults() {
		return "Ctrl+O折叠"
	}
	return "Ctrl+O展开"
}
```

- [ ] **Step 3: Extend renderStatusBar to show shortcut**

In `internal/ui/shell/model.go`, find `renderStatusBar()` function (around line 1498). Modify the right-side section:

Replace the existing renderStatusBar function body with:

```go
func (m Model) renderStatusBar() string {
	var leftParts []string
	var rightParts []string

	// Left: context usage, mode, model name
	if m.runtime.ContextUsage > 0 {
		pct := int(m.runtime.ContextUsage * 100)
		ctxText := styles.ContextStyle(pct).Render(fmt.Sprintf("Context: %d%%", pct))
		leftParts = append(leftParts, ctxText)
	}

	switch m.mode {
	case ModeThinking:
		leftParts = append(leftParts, styles.SystemStyle.Render("Thinking..."))
	case ModeStreaming:
		leftParts = append(leftParts, styles.SystemStyle.Render("Streaming..."))
	case ModeSetup:
		leftParts = append(leftParts, styles.SystemStyle.Render("Setup wizard"))
	}

	if m.deps.ModelName != "" {
		leftParts = append(leftParts, styles.ModelStyle.Render(m.deps.ModelName))
	}

	// Right: shortcut hint + time
	shortcut := m.currentShortcutHint()
	if shortcut != "" {
		rightParts = append(rightParts, styles.HelpStyle.Render(shortcut))
	}
	rightParts = append(rightParts, styles.SystemStyle.Render(time.Now().Format("15:04:05")))

	leftContent := lipgloss.JoinHorizontal(lipgloss.Top, leftParts...)
	rightContent := lipgloss.JoinHorizontal(lipgloss.Top, rightParts...)

	gap := m.width - lipgloss.Width(leftContent) - lipgloss.Width(rightContent)
	if gap < 1 {
		gap = 1
	}

	return leftContent + strings.Repeat(" ", gap) + rightContent
}
```

- [ ] **Step 4: Verify with manual test**

Build and run:
```bash
go build -o fimi ./cmd/fimi && ./fimi
```

Expected: status bar shows "Ctrl+O展开" on right side in idle mode.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/shell/model_output.go internal/ui/shell/model.go
git commit -m "feat(shell): add keyboard shortcut hint to status bar

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 4: config.Save() Function

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing Save test**

In `internal/config/config_test.go`, add at end of file:

```go
func TestSaveFileWritesValidJSON(t *testing.T) {
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "config.json")

	cfg := Default()
	cfg.DefaultModel = "test-model"
	cfg.Models["test-model"] = ModelConfig{
		Provider: "placeholder",
		Model:    "test-model",
	}

	if err := SaveFile(configFile, cfg); err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}

	// Verify file exists and can be loaded
 loaded, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if loaded.DefaultModel != "test-model" {
		t.Fatalf("LoadFile().DefaultModel = %q, want %q", loaded.DefaultModel, "test-model")
	}
}

func TestSaveFileCreatesParentDirectory(t *testing.T) {
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "subdir", "nested", "config.json")

	cfg := Default()
	cfg.DefaultModel = "nested-model"
	cfg.Models["nested-model"] = ModelConfig{
		Provider: "placeholder",
		Model:    "nested-model",
	}

	if err := SaveFile(configFile, cfg); err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}

	loaded, err := LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if loaded.DefaultModel != "nested-model" {
		t.Fatalf("LoadFile().DefaultModel = %q, want %q", loaded.DefaultModel, "nested-model")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/... -run "TestSave" -v
```

Expected: undefined `SaveFile` compilation error.

- [ ] **Step 3: Implement Save and SaveFile**

In `internal/config/config.go`, add after `LoadFile()` function (around line 195):

```go
// Save writes config to the default config file path.
// Uses atomic write: temp file + rename to avoid corruption.
func Save(cfg Config) error {
	configFile, err := File()
	if err != nil {
		return err
	}
	return SaveFile(configFile, cfg)
}

// SaveFile writes config to a specific path.
// Creates parent directories if needed. Uses atomic write pattern.
func SaveFile(path string, cfg Config) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	// Atomic write: temp file + rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write temp config file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename config file %q: %w", path, err)
	}

	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/... -run "TestSave" -v
```

Expected: both tests PASS.

- [ ] **Step 5: Run full config tests**

```bash
go test ./internal/config/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add Save and SaveFile for atomic config writes

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 5: `/setup` Command — Setup Mode and State

**Files:**
- Modify: `internal/ui/shell/model.go`

- [ ] **Step 1: Add ModeSetup constant**

In `internal/ui/shell/model.go`, find the `Mode` enum (around line 23). Add after `ModeCommandSelect`:

```go
// ModeSetup Interactive setup wizard
ModeSetup
```

- [ ] **Step 2: Add setup phase constants and SetupState struct**

In `internal/ui/shell/model.go`, add after `CommandInfo` struct (around line 91):

```go
// setupPhase represents phases in the setup wizard.
type setupPhase int

const (
	setupPhaseWelcome setupPhase = iota
	setupPhaseProviderSelect
	setupPhaseAPIKeyInput
	setupPhaseModelSelect
	setupPhaseSave
)

// SetupState holds temporary state during setup wizard.
type SetupState struct {
	phase            setupPhase
	selectedProvider string
	selectedModel    string
	apiKeyInput      string
	config           config.Config
}
```

Add import:
```go
"fimi-cli/internal/config"
```

- [ ] **Step 3: Add setupState field to Model**

In `internal/ui/shell/model.go`, find the `Model` struct (around line 43). Add field after `showCommandSuggestions`:

```go
// Setup wizard state (active when mode == ModeSetup)
setupState SetupState
```

- [ ] **Step 4: Commit**

```bash
git add internal/ui/shell/model.go
git commit -m "feat(shell): add ModeSetup and SetupState for setup wizard

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 6: `/setup` Command — enterSetupMode and renderSetupView

**Files:**
- Modify: `internal/ui/shell/model.go`

- [ ] **Step 1: Add enterSetupMode function**

In `internal/ui/shell/model.go`, add after `handleCommand()` function (around line 680):

```go
// enterSetupMode initializes the setup wizard.
func (m Model) enterSetupMode() (tea.Model, tea.Cmd) {
	cfg, _ := config.Load() // use defaults if no config exists
	m.mode = ModeSetup
	m.setupState = SetupState{
		phase:  setupPhaseWelcome,
		config: cfg,
	}
	m.showBanner = false
	return m, nil
}
```

- [ ] **Step 2: Add renderSetupView function**

In `internal/ui/shell/model.go`, add after `enterSetupMode()`:

```go
// renderSetupView renders the setup wizard UI.
func (m Model) renderSetupView() string {
	var lines []string

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.ColorPrimary).
		Bold(true)

	promptStyle := lipgloss.NewStyle().
		Foreground(styles.ColorAccent)

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Italic(true)

	switch m.setupState.phase {
	case setupPhaseWelcome:
		lines = append(lines, titleStyle.Render("Setup Wizard"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render("Current configuration:"))
		lines = append(lines, fmt.Sprintf("  Default model: %s", m.setupState.config.DefaultModel))
		lines = append(lines, "")
		if len(m.setupState.config.Models) > 0 {
			lines = append(lines, "  Configured models:")
			for alias, mc := range m.setupState.config.Models {
				lines = append(lines, fmt.Sprintf("    - %s (provider: %s)", alias, mc.Provider))
			}
		} else {
			lines = append(lines, "  No models configured.")
		}
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Press Enter to start setup, Esc/q to cancel"))

	case setupPhaseProviderSelect:
		lines = append(lines, titleStyle.Render("Select Provider"))
		lines = append(lines, "")
		lines = append(lines, "  [1] qwen")
		lines = append(lines, "  [2] openai")
		lines = append(lines, "  [3] anthropic")
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Enter number to select, Esc/q to cancel"))

	case setupPhaseAPIKeyInput:
		lines = append(lines, titleStyle.Render("API Key"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render(fmt.Sprintf("Enter API key for %s:", m.setupState.selectedProvider)))
		// Show asterisks for typed input
		masked := strings.Repeat("*", len(m.setupState.apiKeyInput))
		lines = append(lines, fmt.Sprintf("  %s", masked))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Type key and press Enter, Esc/q to cancel"))

	case setupPhaseModelSelect:
		lines = append(lines, titleStyle.Render("Select Model"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render(fmt.Sprintf("Choose model for %s:", m.setupState.selectedProvider)))
		// Show suggested models for provider
		suggested := suggestedModelsForProvider(m.setupState.selectedProvider)
		for i, model := range suggested {
			lines = append(lines, fmt.Sprintf("  [%d] %s", i+1, model))
		}
		lines = append(lines, "  Or type a custom model name")
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Enter number or custom name, Esc/q to cancel"))

	case setupPhaseSave:
		lines = append(lines, titleStyle.Render("Save Configuration"))
		lines = append(lines, "")
		lines = append(lines, promptStyle.Render("Ready to save:"))
		lines = append(lines, fmt.Sprintf("  Provider: %s", m.setupState.selectedProvider))
		lines = append(lines, fmt.Sprintf("  Model: %s", m.setupState.selectedModel))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Press Enter to save, Esc/q to cancel"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// suggestedModelsForProvider returns common model names for a provider.
func suggestedModelsForProvider(provider string) []string {
	switch provider {
	case "qwen":
		return []string{"qwen-plus", "qwen-turbo", "qwen-max"}
	case "openai":
		return []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}
	case "anthropic":
		return []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"}
	default:
		return nil
	}
}
```

- [ ] **Step 3: Add setup case to handleCommand**

In `internal/ui/shell/model.go`, find `handleCommand()` switch. Add case:

```go
case cmd == "/setup":
    return m.enterSetupMode()
```

- [ ] **Step 4: Add setup view routing in View()**

In `internal/ui/shell/model.go`, find `View()` function (around line 227). Add check after checkpoint select view:

```go
if m.mode == ModeSetup {
    return m.renderSetupView()
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/ui/shell/model.go
git commit -m "feat(shell): add setup wizard entry and view rendering

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 7: `/setup` Command — Keyboard Handler

**Files:**
- Modify: `internal/ui/shell/model.go`

- [ ] **Step 1: Add handleSetupKeyPress function**

In `internal/ui/shell/model.go`, add after `renderSetupView()`:

```go
// handleSetupKeyPress processes keyboard input in setup mode.
func (m Model) handleSetupKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Global cancel
	if keyStr == "esc" || keyStr == "q" {
		m.mode = ModeIdle
		m.setupState = SetupState{}
		return m, nil
	}

	switch m.setupState.phase {
	case setupPhaseWelcome:
		if keyStr == "enter" {
			m.setupState.phase = setupPhaseProviderSelect
		}
		return m, nil

	case setupPhaseProviderSelect:
		switch keyStr {
		case "1":
			m.setupState.selectedProvider = "qwen"
			m.setupState.phase = setupPhaseAPIKeyInput
		case "2":
			m.setupState.selectedProvider = "openai"
			m.setupState.phase = setupPhaseAPIKeyInput
		case "3":
			m.setupState.selectedProvider = "anthropic"
			m.setupState.phase = setupPhaseAPIKeyInput
		}
		return m, nil

	case setupPhaseAPIKeyInput:
		switch keyStr {
		case "enter":
			if m.setupState.apiKeyInput != "" {
				m.setupState.phase = setupPhaseModelSelect
			}
		case "backspace":
			if len(m.setupState.apiKeyInput) > 0 {
				m.setupState.apiKeyInput = m.setupState.apiKeyInput[:len(m.setupState.apiKeyInput)-1]
			}
		default:
			// Accumulate printable characters
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 {
				m.setupState.apiKeyInput += string(msg.Runes)
			}
		}
		return m, nil

	case setupPhaseModelSelect:
		switch keyStr {
		case "enter":
			if m.setupState.selectedModel != "" {
				m.setupState.phase = setupPhaseSave
			}
		case "backspace":
			if len(m.setupState.selectedModel) > 0 {
				m.setupState.selectedModel = m.setupState.selectedModel[:len(m.setupState.selectedModel)-1]
			}
		default:
			// Check for number selection
			suggested := suggestedModelsForProvider(m.setupState.selectedProvider)
			if len(msg.Runes) == 1 {
				num := int(msg.Runes[0] - '0')
				if num >= 1 && num <= len(suggested) {
					m.setupState.selectedModel = suggested[num-1]
					m.setupState.phase = setupPhaseSave
					return m, nil
				}
			}
			// Accumulate printable characters for custom model name
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 {
				m.setupState.selectedModel += string(msg.Runes)
			}
		}
		return m, nil

	case setupPhaseSave:
		if keyStr == "enter" {
			return m.saveSetupConfig()
		}
		return m, nil
	}

	return m, nil
}

// saveSetupConfig writes the setup configuration and returns to idle mode.
func (m Model) saveSetupConfig() (tea.Model, tea.Cmd) {
	// Build provider config
	providerAlias := m.setupState.selectedProvider
	m.setupState.config.Providers[providerAlias] = config.ProviderConfig{
		Type:   providerAlias,
		APIKey: m.setupState.apiKeyInput,
	}
	// Use existing base URL for known providers
	if providerAlias == "qwen" {
		m.setupState.config.Providers[providerAlias].BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}

	// Build model config
	modelAlias := m.setupState.selectedModel
	m.setupState.config.Models[modelAlias] = config.ModelConfig{
		Provider: providerAlias,
		Model:    modelAlias,
	}
	m.setupState.config.DefaultModel = modelAlias

	// Save config
	if err := config.Save(m.setupState.config); err != nil {
		m.mode = ModeIdle
		m.output = m.output.AppendLine(TranscriptLine{
			Type:    LineTypeError,
			Content: fmt.Sprintf("Error saving config: %v", err),
		})
		m.setupState = SetupState{}
		return m, nil
	}

	m.mode = ModeIdle
	m.output = m.output.AppendLine(TranscriptLine{
		Type:    LineTypeSystem,
		Content: "Configuration saved successfully.",
	})
	m.setupState = SetupState{}
	return m, nil
}
```

- [ ] **Step 2: Route setup keypresses in Update()**

In `internal/ui/shell/model.go`, find `Update()` function (around line 141). Add check in `tea.KeyMsg` case before the session/checkpoint checks:

```go
case tea.KeyMsg:
    // Handle setup mode separately
    if m.mode == ModeSetup {
        return m.handleSetupKeyPress(msg)
    }
    // If in session selection mode, special handling
    if m.mode == ModeSessionSelect {
        return m.handleSessionSelectKeyPress(msg)
    }
    // ... rest of existing logic
```

- [ ] **Step 3: Add /setup to availableCommands**

In `internal/ui/shell/model.go`, `availableCommands()`:

```go
{Name: "/setup", Description: "Setup LLM provider and model"},
```

- [ ] **Step 4: Add /setup to helpText**

In `internal/ui/shell/shell.go`, `helpText()`:

```go
"  /setup          Setup LLM provider and model",
```

- [ ] **Step 5: Verify with manual test**

Build and run:
```bash
go build -o fimi ./cmd/fimi && ./fimi
```

Test flow:
1. Type `/setup`
2. Press Enter at welcome
3. Press 1 for qwen
4. Type test API key, Enter
5. Press 1 for qwen-plus
6. Press Enter to save

Expected: "Configuration saved successfully." message.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/shell/model.go internal/ui/shell/shell.go
git commit -m "feat(shell): complete /setup interactive wizard

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 8: Final Verification and Integration

**Files:**
- All modified files

- [ ] **Step 1: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 2: Build and run full smoke test**

```bash
go build -o fimi ./cmd/fimi && ./fimi
```

Test each command:
- `/version` — shows version
- `/release-notes` — shows changelog
- `/setup` — wizard works
- Status bar — shows Ctrl+O hint

- [ ] **Step 3: Update PLAN.md status**

Mark Phase 12 items as complete.

- [ ] **Step 4: Final commit for integration**

```bash
git add PLAN.md
git commit -m "docs: mark Phase 12 shell parity complete

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Self-Review Checklist

**1. Spec coverage:**
- `/version` — Task 1 ✓
- `/release-notes` — Task 2 ✓
- Toolbar hint — Task 3 ✓
- `/setup` — Tasks 4-7 ✓

**2. Placeholder scan:**
- No TBD/TODO found
- All code blocks contain complete implementations
- All test code provided

**3. Type consistency:**
- `ReleaseEntry.Bullets` used consistently
- `SetupState` fields match between definition and usage
- `hasExpandedResults()` method name matches call `HasExpandedResults()`

**Fix:** In Task 3, I wrote `hasExpandedResults()` but called `HasExpandedResults()` — Go naming convention is correct (exported method should be capitalized). The function definition should use `HasExpandedResults()`. Already corrected in the step.

---

Plan complete. Ready for execution.