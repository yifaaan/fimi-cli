# Shell Transcript Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Polish the existing shell transcript so assistant prose, activity groups, spacing, and runtime status presentation match the screenshot direction without changing runtime or tool-preview architecture.

**Architecture:** Keep the existing `TranscriptBlock` pipeline and `DisplayOutput` preview flow intact. Limit the work to transcript presentation: retune transcript-specific colors/styles, flatten activity-group rendering in `model_output.go`, remove the bottom live-status layout integration, and update transcript-focused tests to lock in the new appearance and behavior.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, Go test

---

## File Structure

### Existing files to modify

- `internal/ui/shell/styles/colors.go` — transcript-specific semantic colors such as muted user bubble background, softer transcript foreground, subtler divider tone, and flatter activity accents
- `internal/ui/shell/styles/lipgloss.go` — transcript rendering styles for user bubbles, assistant prose, activity sections, previews, elapsed rows, approvals, and removal of card-oriented transcript styling
- `internal/ui/shell/model_output.go` — transcript block rendering, assistant prose formatting, activity-group flattening, preview indentation, divider/elapsed rendering, and any dead helpers directly tied to the old card treatment
- `internal/ui/shell/model.go:333-365` — main layout assembly; remove bottom live-status section insertion while preserving output-height behavior
- `internal/ui/shell/model_commands.go:339-362` — live-status rendering helpers; remove or shrink them after transcript-first layout no longer uses them
- `internal/ui/shell/model_output_test.go:18-177` — transcript rendering assertions, assistant prose assertions, and transcript polish coverage
- `internal/ui/shell/model_command_test.go:344-373` — replace live-status-specific assertions with transcript/layout assertions that lock in the new behavior

### Files to inspect while implementing

- `internal/ui/shell/transcript_blocks.go` — existing block model that should remain unchanged
- `internal/ui/shell/transcript_builder.go` — only touch if a tiny presentation-supporting adjustment is required
- `docs/superpowers/specs/2026-04-02-shell-transcript-polish-design.md` — approved design source of truth for scope

### No new files expected

This plan should be completed by editing existing shell UI files and tests only.

---

### Task 1: Retune transcript colors and styles

**Files:**
- Modify: `internal/ui/shell/styles/colors.go`
- Modify: `internal/ui/shell/styles/lipgloss.go:22-121`
- Test: `internal/ui/shell/model_output_test.go`

- [ ] **Step 1: Write the failing style-oriented transcript test**

Add or update a renderer-focused test in `internal/ui/shell/model_output_test.go` so it asserts transcript output no longer depends on the old bordered assistant/card treatment. Use a test case shaped like this:

```go
func TestRenderAssistantNoteBlockUsesBulletProse(t *testing.T) {
	rendered := ansi.Strip(renderAssistantNoteBlock("Inspect renderer.\n\nNeed approval parity.", 60))

	for _, want := range []string{
		"• Inspect renderer.",
		"• Need approval parity.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderAssistantNoteBlock() missing %q in %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "│") {
		t.Fatalf("renderAssistantNoteBlock() = %q, want no bordered note treatment", rendered)
	}
}
```

- [ ] **Step 2: Run the targeted test to verify it fails**

Run:

```bash
go test ./internal/ui/shell -run TestRenderAssistantNoteBlockUsesBulletProse -count=1
```

Expected: FAIL because `renderAssistantNoteBlock()` still uses `AssistantBubbleStyle` and does not prefix paragraphs with `•`.

- [ ] **Step 3: Retune transcript color tokens in `colors.go`**

Edit `internal/ui/shell/styles/colors.go` so transcript-specific colors are flatter and more muted. Keep the existing exported names that are used outside transcript rendering, and add only the new semantic tokens needed for transcript-first polish. The change should look like this:

```go
var (
	ColorPrimary  Color = "14"
	ColorSecondary Color = "12"
	ColorBorder   Color = "240"
	ColorTitle    Color = ColorBrightWhite
	ColorMuted    Color = "245"
	ColorAccent   Color = "81"

	ColorTranscriptFg     Color = ColorWhite
	ColorTranscriptMuted  Color = "244"
	ColorTranscriptDivider Color = "239"
	ColorUserBubbleBg     Color = "237"
	ColorActivityExplore  Color = "117"
	ColorActivityCommand  Color = "81"
	ColorActivityEdit     Color = "111"
)
```

Keep the diff/add/remove colors readable; do not change unrelated composer/dropdown/banner colors in this task.

- [ ] **Step 4: Flatten transcript-specific styles in `lipgloss.go`**

Replace the bordered/card-like transcript styles with flatter transcript styles. Update the transcript-related block so it follows this shape:

```go
UserBubbleStyle = lipgloss.NewStyle().
	Foreground(ColorTranscriptFg).
	Background(ColorUserBubbleBg).
	Padding(0, 1)

AssistantBulletStyle = lipgloss.NewStyle().
	Foreground(ColorTranscriptFg)

AssistantBubbleStyle = lipgloss.NewStyle().
	Foreground(ColorTranscriptFg)

ActivityTitleStyle = lipgloss.NewStyle().
	Foreground(ColorTitle).
	Bold(true)

ActivityDetailStyle = lipgloss.NewStyle().
	Foreground(ColorTranscriptMuted)

ActivityPreviewStyle = lipgloss.NewStyle().
	Foreground(ColorTranscriptFg)

ActivityCardStyle = lipgloss.NewStyle()

TranscriptDividerStyle = lipgloss.NewStyle().
	Foreground(ColorTranscriptDivider)

ElapsedStyle = lipgloss.NewStyle().
	Foreground(ColorTranscriptMuted)
```

Important constraints:

- `ActivityCardStyle` should become an effectively flat/no-border transcript container rather than a rounded card
- `AssistantBubbleStyle` should no longer add a left border
- Leave approval, diff, and non-transcript shell styles untouched for now unless they are required to compile

- [ ] **Step 5: Run the transcript style test to verify it passes**

Run:

```bash
go test ./internal/ui/shell -run TestRenderAssistantNoteBlockUsesBulletProse -count=1
```

Expected: PASS.

- [ ] **Step 6: Run the full shell package tests to catch style-related regressions**

Run:

```bash
go test ./internal/ui/shell -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the style retune**

Run:

```bash
git add internal/ui/shell/styles/colors.go internal/ui/shell/styles/lipgloss.go internal/ui/shell/model_output_test.go
git commit -m "refactor(shell): flatten transcript styles"
```

Expected: a new commit that contains only transcript color/style and related test updates.

---

### Task 2: Rework transcript rendering in `model_output.go`

**Files:**
- Modify: `internal/ui/shell/model_output.go:207-415`
- Test: `internal/ui/shell/model_output_test.go:18-177`

- [ ] **Step 1: Write the failing transcript rendering assertions**

Update `TestOutputModelRendersTranscriptBlocks` in `internal/ui/shell/model_output_test.go` so it expects bullet prose and flat transcript sections instead of the old note/card presentation. The key assertions should be:

```go
if !strings.Contains(view, "• Inspecting the current transcript renderer.") {
	t.Fatalf("InteractiveView() missing assistant bullet prose:\n%s", view)
}
if !strings.Contains(view, "• Need inline approval parity.") {
	t.Fatalf("InteractiveView() missing second assistant bullet paragraph:\n%s", view)
}
if strings.Contains(view, "╭") || strings.Contains(view, "╰") {
	t.Fatalf("InteractiveView() still looks card-based:\n%s", view)
}
```

Also add a focused activity-group test with an item + preview body so the old rounded-card rendering fails visibly.

- [ ] **Step 2: Run the targeted rendering tests to verify they fail**

Run:

```bash
go test ./internal/ui/shell -run 'TestOutputModelRendersTranscriptBlocks|TestRenderAssistantNoteBlockUsesBulletProse' -count=1
```

Expected: FAIL because assistant notes are still joined into a plain block and activity groups still render through `activityCardStyle(...).Render(...)`.

- [ ] **Step 3: Rework assistant prose rendering**

Edit `renderAssistantNoteBlock()` in `internal/ui/shell/model_output.go` so each paragraph becomes a bullet paragraph. The implementation should be minimal and keep the current paragraph-splitting helpers. Replace the body assembly with this shape:

```go
func renderAssistantNoteBlock(note string, width int) string {
	paragraphs := splitParagraphs(note)
	if len(paragraphs) == 0 {
		return ""
	}

	rendered := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		rendered = append(rendered, styles.AssistantBulletStyle.Width(width).Render("• "+paragraph))
	}
	return strings.Join(rendered, "\n\n")
}
```

Do not add a new helper unless the existing function would exceed the project’s 50-line preference.

- [ ] **Step 4: Flatten activity-group rendering**

Edit `renderActivityGroupBlock()`, `renderActivityTitle()`, `renderActivityItem()`, `renderPreviewLine()`, and `renderPreviewFooter()` so they render as transcript sections rather than card widgets. The updated rendering should follow this structure:

```go
func (m OutputModel) renderActivityGroupBlock(block TranscriptBlock) string {
	group := block.Activity
	lines := []string{renderActivityTitle(group.Title, group.Accent)}
	for _, item := range group.Items {
		if line := renderActivityItem(item); line != "" {
			lines = append(lines, line)
		}
	}
	if preview := m.renderPreviewBody(block.ID, group.Title, group.Preview); preview != "" {
		lines = append(lines, preview)
	}
	return strings.Join(lines, "\n")
}
```

And the item rendering should stay compact:

```go
prefix := "  · "
switch item.Status {
case ActivityItemRunning:
	prefix = "  › "
case ActivityItemFailed:
	prefix = "  ! "
}
return style.Render(prefix + line)
```

Keep preview indentation compact, for example:

```go
prefix := "    "
```

Do not touch diff-numbering logic. Keep `renderEditDiffPreviewBodyWithWidth(...)` behavior unchanged.

- [ ] **Step 5: Remove dead helpers tied only to the old card treatment**

Delete helpers in `model_output.go` that become unused after the flatter transcript rendering lands. At minimum, re-check these symbols after the refactor and remove them if they are dead:

```go
func activityCardStyle(accent string) lipgloss.Style
func renderActivityBadge(accent string) string
func activityBadgeLabel(accent string) string
func activityBadgeForeground(accent string) styles.Color
```

Do not remove `AppendLine`, `TranscriptLine`, or `transcriptBlockFromLine` in this task; they are still used by system/error/session paths elsewhere in the package.

- [ ] **Step 6: Run the renderer-focused tests to verify they pass**

Run:

```bash
go test ./internal/ui/shell -run 'TestOutputModelRendersTranscriptBlocks|TestRenderAssistantNoteBlockUsesBulletProse|TestRenderEditDiffPreviewBodyKeepsRealHunkContext|TestOutputModelToggleExpandTargetsLatestCollapsiblePreview' -count=1
```

Expected: PASS.

- [ ] **Step 7: Run the full shell package tests**

Run:

```bash
go test ./internal/ui/shell -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit the transcript renderer rewrite**

Run:

```bash
git add internal/ui/shell/model_output.go internal/ui/shell/model_output_test.go
git commit -m "refactor(shell): render transcript as flat sections"
```

Expected: a new commit for assistant prose + activity rendering only.

---

### Task 3: Remove bottom live-status integration

**Files:**
- Modify: `internal/ui/shell/model.go:333-365`
- Modify: `internal/ui/shell/model_commands.go:339-362`
- Test: `internal/ui/shell/model_command_test.go:344-373`

- [ ] **Step 1: Replace live-status tests with transcript-first layout tests**

In `internal/ui/shell/model_command_test.go`, replace the two tests that lock in `renderLiveStatusText()` and `renderLiveStatus()` with tests that enforce removal of the live-status panel from the main layout.

Use a test like this:

```go
func TestMainViewLayoutSectionsDoesNotAppendLiveStatus(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.mode = ModeThinking
	model.runtimeStartedAt = time.Now().Add(-11 * time.Second)

	_, trailing := model.mainViewLayoutSections()
	joined := strings.Join(trailing, "\n")
	if strings.Contains(joined, "Working (") {
		t.Fatalf("mainViewLayoutSections() still includes live status: %q", joined)
	}
}
```

And a helper-level test like this:

```go
func TestRenderOutputForLayoutStillRendersTranscriptWithoutLiveStatus(t *testing.T) {
	model := NewModel(Dependencies{}, nil)
	model.width = 80
	model.height = 24
	model.output = model.output.AppendBlock(TranscriptBlock{Kind: BlockKindUserPrompt, UserText: "hello"})

	got := ansi.Strip(model.renderOutputForLayout(nil, []string{model.input.View()}))
	if !strings.Contains(got, "hello") {
		t.Fatalf("renderOutputForLayout() = %q, want transcript content", got)
	}
}
```

- [ ] **Step 2: Run the targeted layout tests to verify they fail**

Run:

```bash
go test ./internal/ui/shell -run 'TestMainViewLayoutSectionsDoesNotAppendLiveStatus|TestRenderOutputForLayoutStillRendersTranscriptWithoutLiveStatus' -count=1
```

Expected: FAIL because `mainViewLayoutSections()` still appends `renderLiveStatus()`.

- [ ] **Step 3: Remove live-status insertion from `model.go`**

Delete this block from `internal/ui/shell/model.go:342-348`:

```go
if liveStatus := m.renderLiveStatus(); liveStatus != "" {
	trailingSections = append(trailingSections, liveStatus)
}
```

After deleting it, keep the rest of the layout assembly unchanged.

- [ ] **Step 4: Remove or minimize dead live-status helpers in `model_commands.go`**

After the layout stop uses `renderLiveStatus()`, remove the helpers that are now dead if no other caller remains. The likely removals are:

```go
func (m Model) renderLiveStatus() string
func (m Model) renderLiveStatusText() string
func formatRetryLiveStatusText(retry runtimeevents.RetryStatus) string
```

Keep `formatWorkingElapsed(...)` only if another path still calls it after the cleanup. If it becomes dead too, delete it in the same task.

Do not remove unrelated task-list formatting or runtime command helpers from this file.

- [ ] **Step 5: Remove now-unused style definitions if they become dead**

If no code references these styles after the live-status cleanup, delete them from `internal/ui/shell/styles/lipgloss.go`:

```go
StatusBarStyle
LiveStatusStyle
```

Before deleting `StatusBarStyle`, verify `renderStatusBar()` is still unused package-wide.

- [ ] **Step 6: Run the targeted layout tests to verify they pass**

Run:

```bash
go test ./internal/ui/shell -run 'TestMainViewLayoutSectionsDoesNotAppendLiveStatus|TestRenderOutputForLayoutStillRendersTranscriptWithoutLiveStatus' -count=1
```

Expected: PASS.

- [ ] **Step 7: Run the full shell package tests**

Run:

```bash
go test ./internal/ui/shell -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit the transcript-first status cleanup**

Run:

```bash
git add internal/ui/shell/model.go internal/ui/shell/model_commands.go internal/ui/shell/styles/lipgloss.go internal/ui/shell/model_command_test.go
git commit -m "refactor(shell): remove live status panel"
```

Expected: a new commit for layout/status cleanup only.

---

### Task 4: Final transcript regression pass

**Files:**
- Modify: `internal/ui/shell/model_output_test.go`
- Modify: `internal/ui/shell/model_command_test.go`
- Verify: `internal/ui/shell/model_output.go`
- Verify: `internal/ui/shell/model.go`
- Verify: `internal/ui/shell/model_commands.go`

- [ ] **Step 1: Add one end-to-end transcript assertion covering the full polished flow**

Add a final shell transcript test in `internal/ui/shell/model_output_test.go` that exercises:

- user prompt block
- assistant bullet prose
- explored section
- command section
- edit diff section
- elapsed block
- no bottom live-status text in transcript output

Use the block assembly pattern already used by `TestOutputModelRendersTranscriptBlocks`, and add assertions like this:

```go
for _, want := range []string{
	"Refactor shell transcript for screenshot parity",
	"• Inspecting the current transcript renderer.",
	"Explored",
	"Ran pwd && rg --files",
	"Edited PLAN.md (+1 -1)",
	"Worked for 1m 11s",
} {
	if !strings.Contains(view, want) {
		t.Fatalf("InteractiveView() missing %q in:\n%s", want, view)
	}
}
if strings.Contains(view, "Working (") {
	t.Fatalf("InteractiveView() unexpectedly contains removed live status:\n%s", view)
}
```

- [ ] **Step 2: Run the focused regression test to verify it passes**

Run:

```bash
go test ./internal/ui/shell -run TestOutputModelRendersTranscriptBlocks -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the entire shell test suite**

Run:

```bash
go test ./internal/ui/shell -count=1
```

Expected: PASS.

- [ ] **Step 4: Run the broader repo checks most likely to catch shell regressions**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 5: Review touched files for stray dead transcript helpers**

Before finalizing, inspect the touched files and remove any now-unused imports or helpers introduced by the refactor. The minimal cleanup checklist is:

```text
- no unused lipgloss helpers kept only for old activity cards
- no unused live-status helpers or styles remain
- no test still expects plain assistant note blocks or bottom live status
```

- [ ] **Step 6: Commit the final regression lock-in**

Run:

```bash
git add internal/ui/shell/model_output_test.go internal/ui/shell/model_command_test.go internal/ui/shell/model_output.go internal/ui/shell/model.go internal/ui/shell/model_commands.go internal/ui/shell/styles/colors.go internal/ui/shell/styles/lipgloss.go
git commit -m "test(shell): lock transcript polish behavior"
```

Expected: a final commit that captures remaining regression coverage and cleanup.

---

## Self-Review

### Spec coverage

- Rounded-card transcript styling removal: Task 1 and Task 2
- Assistant bullet prose: Task 1 and Task 2
- Remove bottom live-status panel: Task 3
- Tighten transcript spacing / divider / preview density / muted user bubble: Task 1 and Task 2
- Delete obviously dead touched-area helpers: Task 2 and Task 3
- Regression coverage: Task 1 through Task 4

No approved spec requirements are left uncovered.

### Placeholder scan

- No `TODO`, `TBD`, or deferred placeholders remain
- Every task contains exact file paths, code snippets, and commands
- No task references an undefined function or symbol introduced nowhere else in the plan

### Type consistency

- The plan keeps the existing `TranscriptBlock` model unchanged
- It does not rename runtime or tool preview types
- It only removes helpers after verifying package-wide deadness

---

Plan complete and saved to `docs/superpowers/plans/2026-04-02-shell-transcript-polish.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
