# Shell Native Mouse Selection Design

**Goal:** Restore terminal-native mouse behavior in the shell UI so the hosting terminal handles wheel scrolling and drag text selection.

## Problem

The shell currently enables Bubble Tea mouse capture in `internal/ui/shell/shell.go` via `tea.WithMouseCellMotion()`. That causes the terminal to send mouse events to the application instead of keeping native mouse behavior.

For the user, this has two unwanted effects:

1. Mouse wheel behavior is owned by the shell instead of the host terminal.
2. Drag-selecting transcript text is blocked because the terminal is in mouse-reporting mode.

This conflicts with the intended UX: when the shell runs inside Windows Terminal (or another terminal), the terminal itself should provide normal scrollback and native text selection.

## Proposed Design

### 1. Make terminal-native mouse behavior the default

Remove unconditional mouse capture from shell program setup in `internal/ui/shell/shell.go`.

Instead of constructing the Bubble Tea program with `tea.WithMouseCellMotion()`, start it with only the existing input/output wiring. That keeps keyboard-driven shell behavior intact while leaving mouse wheel and drag selection to the host terminal.

### 2. Stop promising shell-owned wheel scrolling

Update shell help text in `internal/ui/shell/shell.go` so it no longer claims:

- `Mouse wheel     Scroll transcript history`

The help text should reflect the new contract:

- wheel scrolling is handled by the terminal
- text selection is handled by the terminal
- shell-specific navigation remains keyboard-based

### 3. Keep keyboard interactions unchanged

Do not change the existing keyboard interaction model:

- `Ctrl+O` still toggles preview expansion
- normal input editing still works
- approval prompts remain keyboard-driven
- transcript rendering logic remains unchanged

This change is intentionally limited to mouse ownership, not transcript behavior.

## Why this approach

### Recommended approach: remove app-level mouse capture entirely

This is the simplest design that exactly matches the requested UX.

Benefits:

- works with terminal-native scrollback
- restores native drag text selection
- avoids terminal-specific mouse edge cases
- matches the comment already present in `shell.go`

Tradeoff:

- shell-owned mouse-wheel transcript scrolling will no longer exist

That tradeoff is acceptable because the requirement explicitly prefers the terminal's default scroll behavior.

## Alternatives considered

### A. Keep Bubble Tea mouse capture and try to allow selection anyway

Rejected.

In practice, once mouse reporting is enabled, terminals usually stop treating drag as native selection. Trying to support both inside the app is terminal-dependent and brittle.

### B. Add a toggle between capture mode and native mode

Rejected.

This adds state, instructions, and complexity without serving the stated goal. The user wants the terminal default behavior all the time.

## Affected Files

- `internal/ui/shell/shell.go`
  - remove `tea.WithMouseCellMotion()` from program setup
  - update help text
- `internal/ui/shell/model.go`
  - likely no functional change required; mouse-message handling can remain inert if no mouse events are produced
- tests under `internal/ui/shell/*_test.go`
  - update/add coverage for the help text and any shell startup assumptions affected by mouse capture removal

## Non-Goals

- no new mouse-driven selection model inside the shell
- no custom copy command
- no transcript rendering changes
- no alt-screen behavior changes beyond current state
- no attempt to preserve shell-owned wheel scrolling

## Testing

### Automated

1. Update help-text assertions so they no longer mention shell-owned mouse-wheel scrolling.
2. Add/adjust shell setup tests if needed to verify shell startup does not request mouse capture.
3. Run the shell package test suite.

### Manual

In Windows Terminal:

1. Launch the shell.
2. Use the mouse wheel and confirm Windows Terminal scrollback moves normally.
3. Drag across transcript text and confirm text can be selected.
4. Verify keyboard-only shell interactions still work:
   - input editing
   - submitting prompts
   - `Ctrl+O`
   - approval selection

## Success Criteria

The design is successful when:

- the shell no longer captures the mouse
- the host terminal handles wheel scrolling
- transcript text can be drag-selected normally
- keyboard-driven shell interactions continue to work unchanged
