# Shell Parity — Design Spec

## Overview

Implement 4 Phase 12 shell features in sequence: `/version`, `/release-notes`, toolbar enhancement, `/setup`.

---

## Feature 1: `/version` Command

### Behavior

When user types `/version`, append a system transcript line displaying the app version.

### Implementation

**Handler** (`model.go` `handleCommand`):
```go
case cmd == "/version":
    m.output = m.output.AppendLine(TranscriptLine{
        Type:    LineTypeSystem,
        Content: versionText(),
    })
    return m, nil
```

**`versionText()`** (new, in `shell.go` or new file):
- Call `app.ResolveAppVersion()` (already exists in `internal/app/version.go`)
- Return formatted string: `fimi-cli v1.2.3` or `fimi-cli dev`

**Command registration** (`availableCommands()`):
```go
{Name: "/version", Description: "Show version information"},
```

**Estimated**: ~15 lines total, all in `model.go` + `shell.go`.

---

## Feature 2: `/release-notes` Command

### Behavior

When user types `/release-notes`, append a system transcript block showing parsed changelog entries. Each entry shows: version tag, date, and bullet points (first 5 per version, then `...` if more). Shows latest 5 versions by default.

### Implementation

**Changelog data**: Embedded as a string constant at build time (`internal/changelog/changelog.go`). The embedded content is the full `CHANGELOG.md` content.

**Parser** (`internal/changelog/parser.go`):
```go
type ReleaseEntry struct {
    Version string
    Date    string
    Sections map[string][]string // "Added" -> [...], "Fixed" -> [...]
}

func ParseReleases(content string, limit int) []ReleaseEntry
```

Parser logic:
1. Split on `## [` to find version headers
2. Extract version number (between `[` and `]`) and date (after `] - `)
3. For each section (`### Added`, `### Changed`, `### Fixed`), collect `- ` bullet lines
4. Return releases in reverse order (newest first), capped at `limit`

**Renderer** (`shell.go`):
```go
func releaseNotesText(entries []changelog.ReleaseEntry) string
```

Renders as:
```
Release Notes:

## [0.35] - 2025-10-22
### Added
  - Minor UI improvements
  - Auto download ripgrep if not found
  - Always approve tool calls in --print mode

## [0.34] - 2025-10-21
### Changed
  - ...
```

**Handler** (`model.go`):
```go
case cmd == "/release-notes":
    entries := changelog.ParseReleases(changelog.Content, 5)
    m.output = m.output.AppendLine(TranscriptLine{
        Type:    LineTypeSystem,
        Content: releaseNotesText(entries),
    })
    return m, nil
```

**Command registration** (`availableCommands()`):
```go
{Name: "/release-notes", Description: "Show release notes"},
```

**Files to create**:
- `internal/changelog/changelog.go` — string constant + `Content` export
- `internal/changelog/parser.go` — parser + types

**Estimated**: ~150 lines total.

---

## Feature 3: Bottom Toolbar Enhancement

### Behavior

Status bar right side (next to clock) shows 2-3 keyboard shortcut hints. In idle mode: `Ctrl+O展开`. In streaming mode: `↵停止`.

### Implementation

**`renderStatusBar()`** already exists in `model.go`. Extend it to add shortcut hints.

```go
func (m Model) renderStatusBar() string {
    var leftParts []string
    var rightParts []string

    // Left: context usage, mode, model name (existing)
    // ...

    // Right: shortcuts + time
    shortcut := m.currentShortcutHint()
    if shortcut != "" {
        rightParts = append(rightParts, styles.HelpStyle.Render(shortcut))
    }
    rightParts = append(rightParts, styles.SystemStyle.Render(time.Now().Format("15:04:05")))

    // Join and pad
    // ...
}
```

**`currentShortcutHint()`**:
```go
func (m Model) currentShortcutHint() string {
    switch m.mode {
    case ModeStreaming:
        return "↵停止"
    case ModeThinking:
        return "↵停止"
    }
    // Idle mode — only show if there are expanded tool results
    if m.output.hasExpandedResults() {
        return "Ctrl+O折叠"
    }
    return "Ctrl+O展开"
}
```

**`hasExpandedResults()`** method on `OutputModel`:
```go
func (m OutputModel) hasExpandedResults() bool {
    for _, v := range m.expanded {
        if v {
            return true
        }
    }
    return false
}
```

**Estimated**: ~30 lines.

---

## Feature 4: `/setup` Command — Interactive Config Wizard

### Behavior

Enters a new Bubble Tea mode (`ModeSetup`) that walks through configuration steps:

1. **Welcome** — show current config state, ask whether to proceed
2. **Model Selection** — choose from configured models, or add new
3. **Provider Confirmation** — confirm/edit provider settings
4. **Save** — write config and return to shell

Each step renders in the transcript area, input is used for responses.

### Mode Design

```
ModeSetup SetupPhase
where SetupPhase = phaseWelcome | phaseModelSelect | phaseProviderConfirm | phaseSave | phaseDone
```

**`handleSetupPhase`** handles input per phase:
- `phaseWelcome`: "y" → next; any key → cancel
- `phaseModelSelect`: show model list, number input → select; "n" → add model
- `phaseProviderConfirm`: show provider config, "y" → save; other → cancel
- `phaseSave`: write config, confirm, return to idle
- `phaseDone`: return to idle

**UI Rendering** (`renderSetupView()`):
- Shows prompt text in transcript style
- Shows available choices as numbered options
- Input field shows typed response

**Config writing** (`internal/config/config.go`):
- `Save(cfg Config) error` — write to default file path, atomic via temp file + rename

**Handler** (`model.go`):
```go
case cmd == "/setup":
    return m.enterSetupMode()
```

**Files to create/modify**:
- `internal/config/config.go` — add `Save()` method
- `model.go` — add `ModeSetup` enum, `setupPhase` state, `handleSetupPhase`, `renderSetupView`, `enterSetupMode`
- `shell.go` — add setup help text to `helpText()`

**Estimated**: ~250 lines.

---

## File Summary

| File | Change |
|------|--------|
| `internal/ui/shell/model.go` | Add `/version`, `/release-notes`, `/setup` cases in `handleCommand`. Add `ModeSetup`, setup state, handlers, renderer. Extend `renderStatusBar`. |
| `internal/ui/shell/shell.go` | Add `versionText()`, `releaseNotesText()`, setup help lines. |
| `internal/ui/shell/model_output.go` | Add `hasExpandedResults()` to `OutputModel`. |
| `internal/changelog/changelog.go` | New — embedded CHANGELOG content constant. |
| `internal/changelog/parser.go` | New — `ReleaseEntry`, `ParseReleases()`. |
| `internal/config/config.go` | Add `Save()` method. |

## Build Integration

The `internal/changelog/changelog.go` content should be embedded via `//go:embed` directive:

```go
//go:embed CHANGELOG.md
var changelogContent string
```

Place `CHANGELOG.md` (copy from `temp/CHANGELOG.md`) next to the Go file.
