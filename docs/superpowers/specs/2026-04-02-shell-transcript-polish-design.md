# Shell Transcript Polish Design

**Date:** 2026-04-02
**Status:** Draft for review

## Goal

Polish the existing shell transcript presentation so it more closely matches the screenshot target without reopening runtime or tool-preview architecture work.

This pass should handle four concrete changes together:

1. remove the remaining rounded-card / generic shell styling from transcript activity blocks
2. switch assistant notes to screenshot-style bullet prose
3. remove the bottom live-status panel entirely
4. tighten transcript spacing, divider rhythm, preview density, and muted user-bubble treatment

## Scope

### In scope

- Shell transcript rendering and transcript-specific styles
- Transcript block spacing and divider rhythm
- Assistant narrative presentation
- Activity-group presentation
- User prompt bubble styling
- Removal of the bottom live-status panel from the main shell layout
- Deletion of obviously dead legacy line-based helpers encountered in the touched area
- Regression tests for the changed shell transcript behavior

### Out of scope

- Runtime event schema changes
- Tool execution changes
- Tool `DisplayOutput` plumbing redesign
- New grouping rules unless required by presentation cleanup
- Print UI redesign
- Persisted history format changes
- Broader shell layout redesign unrelated to transcript-first presentation

## Current State

The transcript architecture refactor is already in place.

The shell already has:

- `TranscriptBlock` and related block kinds in `internal/ui/shell/transcript_blocks.go`
- grouped runtime-to-transcript mapping in `internal/ui/shell/transcript_builder.go`
- block rendering in `internal/ui/shell/model_output.go`
- richer tool previews via `DisplayOutput` in builtin tools
- inline elapsed and approval blocks
- regression coverage for transcript rendering and preview behavior

The main remaining mismatch is presentation polish. The current shell still mixes the new transcript model with older shell-chrome assumptions:

- activity groups still look like rounded cards
- assistant notes are still rendered as plain note blocks instead of bullet prose
- a separate bottom live-status panel still appears during runs
- transcript spacing and muted styling are not yet tuned to the screenshot target

## Design Summary

Keep the existing transcript block model and tool preview pipeline intact. Perform a transcript-first presentation pass that narrows all visible work to the shell rendering layer.

The design keeps three boundaries stable:

- runtime events remain unchanged
- transcript grouping rules remain mostly unchanged
- tool preview generation remains unchanged

The work is concentrated in shell presentation files so that the screenshot-parity pass stays low risk and easy to verify.

## File-Level Design

### `internal/ui/shell/model_output.go`

This file becomes the center of the polish pass.

#### Changes

- Rework `renderAssistantNoteBlock()` to render paragraph-based bullet prose
- Rework `renderActivityGroupBlock()` so activity sections render as flatter transcript sections instead of card widgets
- Reduce container styling around previews so previews feel attached to the transcript flow rather than to separate output boxes
- Tighten spacing between title, items, preview body, divider, and elapsed blocks
- Keep existing expand/collapse and diff preview behavior intact
- Remove clearly dead legacy helpers in this file when the new transcript path fully replaces them

#### Intended rendering behavior

- Assistant paragraphs render as lines prefixed with `•`
- Activity sections rely on title weight, indentation, muted detail rows, and divider rhythm rather than rounded borders
- User prompts remain visually distinct but softer and more muted than the current generic shell bubble
- Diff previews keep the current semantic coloring but inherit the flatter transcript layout

### `internal/ui/shell/styles/colors.go`

Introduce or retune transcript-focused semantic colors.

#### Changes

- Add a more muted user-bubble background token
- Add softer transcript foreground and muted foreground tokens if current values are too shell-generic
- Add a subtler divider tone
- Retune activity accents for explore / command / edit states to work in flatter transcript sections
- Keep diff add/remove colors readable without looking like standalone cards

### `internal/ui/shell/styles/lipgloss.go`

Replace card-oriented transcript styles with flatter transcript styles.

#### Changes

- Rework `UserBubbleStyle`
- Replace bordered assistant note treatment with bullet-prose styling
- Rework activity title, detail, preview, divider, and elapsed styles for transcript-first presentation
- Remove rounded-border assumptions from activity-group rendering styles used in transcript output
- Keep approval rendering inline, but visually closer to transcript sections than to floating cards where practical

### `internal/ui/shell/model.go`

Remove the bottom live-status panel from the main shell layout.

#### Changes

- Stop appending `renderLiveStatus()` into the main view layout
- Recheck output-height calculation after removing that layout section
- Preserve scrolling, pending-turn display, and interactive-tail behavior

### `internal/ui/shell/model_commands.go`

Shrink logic that only exists for the removed live-status panel.

#### Changes

- Remove or minimize `renderLiveStatus()` and any helper logic that becomes dead once the panel is gone
- Keep only reusable retry/progress text helpers if something else still depends on them
- Avoid introducing a new replacement panel; this pass intentionally moves to transcript-first progress presentation

### `internal/ui/shell/transcript_builder.go`

Keep changes minimal.

#### Changes

- Only touch this file if assistant prose formatting or transcript-first progress requires a small supporting adjustment
- Do not redesign grouping logic during this pass

## Rendering Rules

### User prompt block

- Full-width transcript block within the transcript panel width
- Muted background
- No explicit role label
- Padding should remain modest so the transcript stays dense

### Assistant note block

- Render each logical paragraph as a bullet paragraph
- Preserve paragraph breaks
- No border or assistant role label
- Text should read like narrative guidance in the transcript stream

### Activity group block

- Render title first, then detail items, then preview body
- Remove rounded-card framing from transcript activity sections
- Use indentation and typography rather than box chrome to show hierarchy
- Keep accent only where it helps orientation; do not let accent dominate the transcript

### Preview body

- Preview stays directly attached to the owning activity section
- Keep expand/collapse behavior and hints
- Maintain current diff numbering and diff semantic coloring
- Tighten preview density so previews feel transcript-native rather than like separate panes

### Divider and elapsed blocks

- Divider should act as a rhythm marker, not as heavy decoration
- Elapsed block remains a lightweight transcript line such as `Worked for 1m 11s`
- Spacing around divider and elapsed blocks should be consistent with transcript flow

## Dead-Code Cleanup Rule

If the touched presentation path fully replaces legacy line-based helpers, delete the clearly dead helpers in the same pass.

Constraints:

- only remove code that is clearly unused after the presentation rewrite
- do not broaden cleanup into unrelated shell code
- prefer deleting obsolete compatibility helpers over keeping misleading duplicate paths

## Testing Strategy

### Update existing tests

Primary files:

- `internal/ui/shell/model_output_test.go`
- `internal/ui/shell/model_command_test.go`

### Required assertions

- assistant notes render as bullet prose
- transcript output no longer includes the bottom live-status panel
- activity groups no longer render with rounded-card assumptions in transcript view
- user prompt remains visually distinct
- expand/collapse still works for long previews
- diff previews still preserve numbering and context behavior
- scrolling and pending/committed transcript behavior continue to work

### Nice-to-have additions

- one transcript-focused golden assertion covering user block + assistant prose + explored block + command block + edit diff without the live-status panel

## Risks

### 1. Wrapping and indentation regressions

Flattening activity sections and bullet prose can change terminal wrapping behavior.

**Mitigation:** keep changes localized to renderer functions and verify output using existing transcript rendering tests.

### 2. Loss of run-state visibility

Removing the live-status panel could make active runs feel less obvious.

**Mitigation:** rely on transcript activity, approval blocks, elapsed markers, and existing runtime updates already present in the stream. This pass intentionally prefers transcript-first visibility over duplicated status chrome.

### 3. Over-cleaning legacy helpers

Deleting compatibility helpers too aggressively could break edge paths.

**Mitigation:** only delete helpers proven dead within the touched files and covered paths.

## Recommended Implementation Order

1. Rework transcript styles in `colors.go` and `lipgloss.go`
2. Rework assistant note and activity group rendering in `model_output.go`
3. Remove bottom live-status layout integration from `model.go`
4. Delete or minimize obsolete live-status helpers in `model_commands.go`
5. Remove obviously dead line-based helpers encountered in the touched rendering path
6. Update transcript rendering tests and command/layout tests

## Acceptance Criteria

- Assistant narrative renders as bullet prose in the transcript
- Activity sections no longer look like rounded tool cards
- Bottom live-status panel is removed from the shell layout
- User prompt bubble is visibly muted and transcript-native
- Transcript spacing and divider rhythm are visibly closer to the screenshot target
- Existing transcript capabilities keep working: grouped activity, inline approval, elapsed blocks, diff preview expand/collapse, scrolling, and pending-turn behavior

## Non-Goals for This Pass

This pass does not attempt exact pixel-perfect screenshot parity. It is a transcript-polish and transcript-first cleanup pass built on the already-completed block architecture.
