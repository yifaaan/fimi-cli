# Shell Grouped Preview — Design Spec

## Overview

Fix the shell transcript so grouped `Explored` activity cards do not lose earlier tool results when multiple tools run in sequence. The current implementation stores only one group-level preview, so later tool results overwrite earlier ones. The new behavior keeps the compact grouped list in the collapsed state, and when expanded shows each item's preview in order.

## Problem

Today, consecutive `read_file`, `glob`, `grep`, `search_web`, and `fetch_url` tool calls are grouped into a single `Explored` card. That card stores preview data at the group level. When a second tool result arrives, it replaces the first preview. As a result, only the most recent preview is visible even though the item list shows multiple tool calls.

This is most visible when two or more `Read` calls happen back-to-back: the card lists both files, but only one preview appears.

## Goals

- Keep the existing grouped `Explored` card layout.
- Preserve the compact collapsed state that only shows the item list.
- When the card is expanded, show each item's preview in item order.
- Preserve existing diff rendering for edit-like tools.
- Avoid breaking older persisted transcript records that only have group-level preview data.

## Non-Goals

- No new keyboard shortcuts.
- No per-item focus or selection model.
- No redesign of command/edit cards.
- No change to grouping rules for exploration tools.

## Recommended Approach

Store preview data on each `ActivityItem`, keep a group-level aggregated collapsible state, and render item previews only when the card is expanded.

This keeps the current codex-like compact grouping while fixing the data-loss behavior. It avoids introducing cursor/focus state and keeps `Ctrl+O` semantics unchanged.

## Alternatives Considered

### 1. Per-item previews inside the existing grouped card (recommended)

Keep the `Explored` card grouped. In collapsed mode, show only the item list. In expanded mode, render every item's preview sequentially after the list.

**Pros**
- Fixes the missing-preview bug.
- Preserves the current grouped look.
- Minimal interaction changes.

**Cons**
- Requires moving some preview state from group level to item level.

### 2. Single active preview with item focus

Keep one visible preview at a time and add focus navigation among items.

**Pros**
- Compact expanded state.

**Cons**
- Adds new state and interaction complexity.
- Solves a problem the user did not ask for.

### 3. Split each tool call into its own card

Stop reusing the `Explored` group and render every tool independently.

**Pros**
- Simple data model.

**Cons**
- Loses the grouped transcript style.
- Makes the transcript noisier.

## Interaction Design

### Collapsed state

An `Explored` card continues to render:
- the `EXPLORED` badge and title
- the ordered item list (`Read ...`, `Search ...`)
- one footer hint when the group contains collapsible preview content

Collapsed state does **not** show any preview body.

### Expanded state

When the user presses `Ctrl+O` on the latest collapsible block:
- the same title and item list remain visible
- after the list, render each item preview in the same order as the items
- each preview section is labeled with the associated item text so the user can tell which result belongs to which tool call
- the expand/collapse hint is rendered once for the whole card, not once per item

### Keyboard behavior

`Ctrl+O` behavior stays the same:
- it targets the latest collapsible block
- it toggles the whole grouped card between collapsed and expanded

No new navigation behavior is introduced.

## Data Model Design

### Activity item preview ownership

`ActivityItem` should become the primary owner of preview content for grouped exploration tools.

Each item may carry:
- preview text
- preview kind (`text` or `diff`)
- whether that item's preview is collapsible
- an optional preview label derived from the item verb/text or tool summary

### Group-level state

`ActivityGroupBlock` should continue to expose group-level collapse behavior for rendering and `Ctrl+O` targeting.

The group remains collapsible when at least one contained preview is collapsible, or more simply when any preview content exists that should participate in grouped expansion.

For backward compatibility, the existing group-level `Preview` field can remain temporarily as a fallback for legacy records and non-itemized groups.

## Transcript Builder Changes

### Tool call handling

The current grouping logic for exploration tools remains unchanged. Consecutive exploration calls still append items into one `Explored` group.

### Tool result handling

When a tool result arrives for an itemized group:
- resolve the referenced `ActivityItem`
- compute normalized preview text exactly as today
- classify preview kind exactly as today
- store the preview on that item instead of overwriting the group-level preview
- update the group's aggregate `Collapsible` state

For non-itemized groups such as command/edit cards, existing group-level preview behavior can remain.

## Rendering Design

### Group rendering flow

`renderActivityGroupBlock` should render in this order:
1. group title
2. item list
3. fallback group-level preview when applicable
4. item-level previews when expanded for grouped exploration cards
5. one shared footer hint

The grouped exploration path should suppress body previews while collapsed.

### Item preview rendering

Each expanded item preview section should:
- start with a short label derived from the item, such as `Read internal/tools/builtin.go`
- render its body using the existing preview rendering paths
- preserve diff/text styling rules already in use elsewhere

### Footer behavior

Only one expand/collapse footer should appear per card.

In collapsed mode it should summarize hidden content for the group. In expanded mode it should still show the single `Ctrl+O collapse` hint.

## Backward Compatibility

Older transcripts or resumed sessions may contain only group-level preview data. The renderer should keep supporting that shape.

Compatibility rule:
- if item previews are present, prefer them for grouped exploration cards
- otherwise fall back to the existing group-level `Preview`

This prevents older history from rendering blank previews.

## Testing Strategy

Follow TDD.

### Required failing test first

Add a regression test that reproduces the bug with one grouped `Explored` card containing at least two sequential `read_file` results.

Expected behavior:
- collapsed view shows the grouped list but not preview bodies
- expanded view shows both previews in order
- each preview is associated with the correct item label
- no earlier preview is lost when the later result arrives

### Additional coverage

Add or update tests for:
- mixed grouped exploration items (`read_file` + `glob` + `grep`)
- legacy group-level preview fallback still rendering
- `Ctrl+O` still targeting the latest collapsible block
- non-exploration cards (`command`, `edit`) preserving current behavior

## Risks

- Rendering may duplicate footer hints if item preview rendering is not separated cleanly from existing group preview rendering.
- Resume/history code may carry older block shapes, so fallback behavior must be kept until transcripts are fully itemized.
- The existing assumption that a group has at most one preview appears in both builder and renderer; both sides must be updated together.

## Implementation Notes

Keep the change minimal:
- preserve current grouping
- preserve current keyboard behavior
- preserve current diff renderer
- avoid introducing helper abstractions unless needed by repeated logic

The core fix is to stop treating grouped exploration results as a single preview-bearing unit and instead treat each listed activity item as the owner of its own preview content.
