# Shell Grouped Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve every grouped exploration tool result in the shell transcript and show each item's preview in order when the card is expanded.

**Architecture:** Keep grouped `Explored` cards, move preview ownership from the group to `ActivityItem`, and aggregate expand/collapse behavior at the group level. Update the transcript builder to attach previews to the correct item, then update rendering so grouped exploration cards stay compact while collapsed and show labeled per-item previews when expanded, with legacy fallback for older group-level previews.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing shell transcript builder/renderer tests.

---

## File Structure

| File | Purpose |
|------|---------|
| `internal/ui/shell/transcript_blocks.go` | Extend transcript data structures so grouped items can own previews while preserving legacy group-level fallback. |
| `internal/ui/shell/transcript_builder.go` | Attach tool results to the correct grouped item instead of overwriting a single group preview; keep aggregate expandable state updated. |
| `internal/ui/shell/model_output.go` | Render grouped exploration cards with item-level previews only when expanded; keep one shared footer and legacy fallback. |
| `internal/ui/shell/model_output_test.go` | Regression tests for grouped exploration preview behavior, expand/collapse behavior, builder behavior, and legacy fallback. |

---

### Task 1: Add a failing rendering regression test for sequential grouped reads

**Files:**
- Modify: `internal/ui/shell/model_output_test.go`
- Test: `internal/ui/shell/model_output_test.go`

- [ ] **Step 1: Write the failing test**

Add this test near the other transcript rendering tests in `internal/ui/shell/model_output_test.go`:

```go
func TestOutputModelExpandedExploredGroupShowsEachItemPreviewInOrder(t *testing.T) {
	model := NewOutputModel()
	model.width = 100
	model.height = 40
	model = model.AppendBlock(TranscriptBlock{
		ID:   "explored-seq",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind:   "explored",
			Title:       "Explored",
			Collapsible: true,
			Items: []ActivityItem{
				{
					ToolCallID: "read-1",
					Verb:       "Read",
					Text:       "internal/tools/builtin.go",
					Status:     ActivityItemCompleted,
					Preview: PreviewBody{
						Text:        "package tools\nfunc Builtin() {}",
						Kind:        PreviewKindText,
						Collapsible: false,
					},
				},
				{
					ToolCallID: "read-2",
					Verb:       "Read",
					Text:       "internal/tools/builtin_readonly.go",
					Status:     ActivityItemCompleted,
					Preview: PreviewBody{
						Text:        "package tools\nfunc Readonly() {}",
						Kind:        PreviewKindText,
						Collapsible: false,
					},
				},
			},
		},
	})

	collapsed := model.InteractiveView()
	for _, want := range []string{
		"Explored",
		"Read internal/tools/builtin.go",
		"Read internal/tools/builtin_readonly.go",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("collapsed InteractiveView() missing %q in:\n%s", want, collapsed)
		}
	}
	for _, unwanted := range []string{
		"func Builtin() {}",
		"func Readonly() {}",
	} {
		if strings.Contains(collapsed, unwanted) {
			t.Fatalf("collapsed InteractiveView() unexpectedly contains %q in:\n%s", unwanted, collapsed)
		}
	}

	updated, toggled := model.ToggleExpand()
	if !toggled {
		t.Fatal("ToggleExpand() = false, want grouped explored block to expand")
	}

	expanded := updated.InteractiveView()
	for _, want := range []string{
		"Read internal/tools/builtin.go",
		"package tools",
		"func Builtin() {}",
		"Read internal/tools/builtin_readonly.go",
		"func Readonly() {}",
	} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded InteractiveView() missing %q in:\n%s", want, expanded)
		}
	}
	if strings.Index(expanded, "func Builtin() {}") > strings.Index(expanded, "func Readonly() {}") {
		t.Fatalf("expanded InteractiveView() rendered previews out of order:\n%s", expanded)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/ui/shell -run TestOutputModelExpandedExploredGroupShowsEachItemPreviewInOrder -count=1
```

Expected: FAIL with a compile error because `ActivityItem` does not yet have a `Preview` field.

---

### Task 2: Add item-level preview support to transcript blocks

**Files:**
- Modify: `internal/ui/shell/transcript_blocks.go`
- Test: `internal/ui/shell/model_output_test.go`

- [ ] **Step 1: Extend `ActivityItem` with preview data**

In `internal/ui/shell/transcript_blocks.go`, replace the `ActivityItem` definition with:

```go
type ActivityItem struct {
	ToolCallID string
	Verb       string
	Text       string
	Status     ActivityItemStatus
	Preview    PreviewBody
}
```

- [ ] **Step 2: Update collapsible detection to support item previews**

In `internal/ui/shell/transcript_blocks.go`, replace the imports and `IsCollapsible()` with:

```go
import (
	"strings"
	"time"
)

func (b TranscriptBlock) IsCollapsible() bool {
	switch b.Kind {
	case BlockKindActivityGroup:
		if !b.Activity.Collapsible {
			return false
		}
		if strings.TrimSpace(b.Activity.Preview.Text) != "" {
			return true
		}
		for _, item := range b.Activity.Items {
			if strings.TrimSpace(item.Preview.Text) != "" {
				return true
			}
		}
		return false
	default:
		return false
	}
}
```

- [ ] **Step 3: Run the regression test to verify the compile error is gone and the behavior still fails**

Run:

```bash
go test ./internal/ui/shell -run TestOutputModelExpandedExploredGroupShowsEachItemPreviewInOrder -count=1
```

Expected: FAIL at runtime because rendering still does not show item previews after expansion.

- [ ] **Step 4: Commit the data model change**

Run:

```bash
git add internal/ui/shell/transcript_blocks.go internal/ui/shell/model_output_test.go
git commit -m "refactor(shell): store previews on activity items"
```

Expected: commit succeeds with the data-model change and the regression test now compiling.

---

### Task 3: Attach grouped tool results to the correct activity item

**Files:**
- Modify: `internal/ui/shell/transcript_builder.go`
- Modify: `internal/ui/shell/model_output_test.go`
- Test: `internal/ui/shell/model_output_test.go`

- [ ] **Step 1: Write a builder-level failing test for sequential grouped reads**

Add this test near `TestRuntimeModelGroupsExplorationToolsAndSkipsThoughtLoggedResult` in `internal/ui/shell/model_output_test.go`:

```go
func TestRuntimeModelStoresPreviewOnEachExploredItem(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-1",
		Name:      "read_file",
		Subtitle:  "Read internal/tools/builtin.go",
		Arguments: `{"path":"internal/tools/builtin.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-1",
		ToolName:      "read_file",
		Output:        "package tools\nfunc Builtin() {}",
		DisplayOutput: "package tools\nfunc Builtin() {}",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-2",
		Name:      "read_file",
		Subtitle:  "Read internal/tools/builtin_readonly.go",
		Arguments: `{"path":"internal/tools/builtin_readonly.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-2",
		ToolName:      "read_file",
		Output:        "package tools\nfunc Readonly() {}",
		DisplayOutput: "package tools\nfunc Readonly() {}",
	})

	blocks := model.ToBlocks()
	if len(blocks) != 1 {
		t.Fatalf("len(ToBlocks()) = %d, want 1", len(blocks))
	}

	explored := blocks[0]
	if len(explored.Activity.Items) != 2 {
		t.Fatalf("len(explored.Activity.Items) = %d, want 2", len(explored.Activity.Items))
	}
	if explored.Activity.Items[0].Preview.Text == "" {
		t.Fatal("items[0].Preview.Text is empty, want first read preview preserved")
	}
	if explored.Activity.Items[1].Preview.Text == "" {
		t.Fatal("items[1].Preview.Text is empty, want second read preview preserved")
	}
	if strings.Contains(explored.Activity.Items[0].Preview.Text, "Readonly") {
		t.Fatalf("items[0].Preview.Text = %q, want first item to keep first preview", explored.Activity.Items[0].Preview.Text)
	}
	if !strings.Contains(explored.Activity.Items[1].Preview.Text, "Readonly") {
		t.Fatalf("items[1].Preview.Text = %q, want second item to keep second preview", explored.Activity.Items[1].Preview.Text)
	}
}
```

- [ ] **Step 2: Run the builder test to verify it fails**

Run:

```bash
go test ./internal/ui/shell -run TestRuntimeModelStoresPreviewOnEachExploredItem -count=1
```

Expected: FAIL because `applyToolResult` still writes to `block.Activity.Preview` instead of the item.

- [ ] **Step 3: Update grouped tool-result handling**

In `internal/ui/shell/transcript_builder.go`, replace the preview assignment block inside `applyToolResult` with:

```go
	previewText := normalizePreviewText(block.Activity.Title, toolResultDisplayOutput(result.Output, result.DisplayOutput))
	if strings.TrimSpace(previewText) == "" {
		return
	}
	previewKind := classifyPreviewKind(toolName, previewText)
	preview := PreviewBody{
		Text:        previewText,
		Kind:        previewKind,
		Collapsible: previewLineCount(previewText) > previewDefaultLimit(previewKind),
	}

	if ref.itemIdx >= 0 {
		block.Activity.Items[ref.itemIdx].Preview = preview
		block.Activity.Collapsible = true
		return
	}

	block.Activity.Preview = preview
	block.Activity.Collapsible = block.Activity.Preview.Collapsible
```

- [ ] **Step 4: Run the builder tests to verify they pass**

Run:

```bash
go test ./internal/ui/shell -run 'TestRuntimeModelStoresPreviewOnEachExploredItem|TestRuntimeModelGroupsExplorationToolsAndSkipsThoughtLoggedResult' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the builder change**

Run:

```bash
git add internal/ui/shell/transcript_builder.go internal/ui/shell/model_output_test.go
git commit -m "fix(shell): preserve previews for grouped explore items"
```

Expected: commit succeeds with builder logic and test coverage.

---

### Task 4: Render grouped exploration previews only when expanded

**Files:**
- Modify: `internal/ui/shell/model_output.go`
- Modify: `internal/ui/shell/model_output_test.go`
- Test: `internal/ui/shell/model_output_test.go`

- [ ] **Step 1: Add a failing fallback test for legacy group previews**

Add this test near the rendering tests in `internal/ui/shell/model_output_test.go`:

```go
func TestOutputModelExploredGroupFallsBackToLegacyGroupPreview(t *testing.T) {
	model := NewOutputModel()
	model.width = 100
	model.height = 40
	model = model.AppendBlock(TranscriptBlock{
		ID:   "legacy-explored",
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind:   "explored",
			Title:       "Explored",
			Collapsible: true,
			Items: []ActivityItem{
				{Verb: "Read", Text: "internal/tools/builtin.go", Status: ActivityItemCompleted},
			},
			Preview: PreviewBody{
				Text:        "package tools\nfunc Legacy() {}",
				Kind:        PreviewKindText,
				Collapsible: false,
			},
		},
	})

	collapsed := model.InteractiveView()
	if strings.Contains(collapsed, "func Legacy() {}") {
		t.Fatalf("collapsed InteractiveView() unexpectedly shows legacy preview:\n%s", collapsed)
	}

	updated, toggled := model.ToggleExpand()
	if !toggled {
		t.Fatal("ToggleExpand() = false, want legacy explored block to expand")
	}

	expanded := updated.InteractiveView()
	for _, want := range []string{"Read internal/tools/builtin.go", "func Legacy() {}"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded InteractiveView() missing %q in:\n%s", want, expanded)
		}
	}
}
```

- [ ] **Step 2: Run the rendering tests to verify they fail**

Run:

```bash
go test ./internal/ui/shell -run 'TestOutputModelExpandedExploredGroupShowsEachItemPreviewInOrder|TestOutputModelExploredGroupFallsBackToLegacyGroupPreview' -count=1
```

Expected: FAIL because the renderer still only knows how to show one group preview.

- [ ] **Step 3: Add grouped exploration rendering helpers**

In `internal/ui/shell/model_output.go`, add these helpers after `renderActivityItem`:

```go
func isGroupedExplorationCard(group ActivityGroupBlock) bool {
	return strings.TrimSpace(group.GroupKind) == "explored" && len(group.Items) > 0
}

func activityItemLabel(item ActivityItem) string {
	return strings.TrimSpace(strings.Join([]string{item.Verb, item.Text}, " "))
}

func groupedExplorationHiddenLineCount(group ActivityGroupBlock) int {
	hidden := 0
	for _, item := range group.Items {
		text := strings.TrimSpace(item.Preview.Text)
		if text == "" {
			continue
		}
		hidden += previewLineCount(text)
	}
	if hidden > 0 {
		return hidden
	}
	return previewLineCount(group.Preview.Text)
}

func (m OutputModel) renderGroupedExplorationPreviews(group ActivityGroupBlock) []string {
	var sections []string
	for _, item := range group.Items {
		if strings.TrimSpace(item.Preview.Text) == "" {
			continue
		}
		label := activityItemLabel(item)
		if label != "" {
			sections = append(sections, styles.ActivityDetailStyle.Render("    "+label))
		}
		for _, line := range strings.Split(strings.TrimRight(item.Preview.Text, "\n"), "\n") {
			sections = append(sections, renderPreviewLine(item.Preview.Kind, line))
		}
	}
	if len(sections) > 0 {
		return sections
	}
	if body := m.renderPreviewBody("", group.Title, PreviewBody{
		Text:        group.Preview.Text,
		Kind:        group.Preview.Kind,
		Collapsible: false,
	}); body != "" {
		return []string{body}
	}
	return nil
}
```

- [ ] **Step 4: Update grouped card rendering to use the new helpers**

In `internal/ui/shell/model_output.go`, replace `renderActivityGroupBlock` with:

```go
func (m OutputModel) renderActivityGroupBlock(block TranscriptBlock) string {
	group := block.Activity
	lines := []string{renderActivityTitle(group.Title, group.Accent)}
	for _, item := range group.Items {
		lines = append(lines, renderActivityItem(item))
	}

	if isGroupedExplorationCard(group) {
		if m.expanded[block.ID] {
			if previews := m.renderGroupedExplorationPreviews(group); len(previews) > 0 {
				lines = append(lines, previews...)
			}
			lines = append(lines, renderPreviewFooter("    ", PreviewKindText, 0, "", previewToggleHint(true)))
			return activityCardStyle(group.Accent).Width(m.panelWidth()).Render(strings.Join(lines, "\n"))
		}

		hidden := groupedExplorationHiddenLineCount(group)
		if hidden > 0 {
			lines = append(lines, renderPreviewFooter("    ", PreviewKindText, hidden, "lines", previewToggleHint(false)))
		}
		return activityCardStyle(group.Accent).Width(m.panelWidth()).Render(strings.Join(lines, "\n"))
	}

	if preview := m.renderPreviewBody(block.ID, group.Title, group.Preview); preview != "" {
		lines = append(lines, preview)
	}
	return activityCardStyle(group.Accent).Width(m.panelWidth()).Render(strings.Join(lines, "\n"))
}
```

- [ ] **Step 5: Run the targeted rendering tests to verify they pass**

Run:

```bash
go test ./internal/ui/shell -run 'TestOutputModelExpandedExploredGroupShowsEachItemPreviewInOrder|TestOutputModelExploredGroupFallsBackToLegacyGroupPreview|TestOutputModelToggleExpandTargetsLatestCollapsiblePreview' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the rendering change**

Run:

```bash
git add internal/ui/shell/model_output.go internal/ui/shell/model_output_test.go
git commit -m "fix(shell): render grouped explore previews on expand"
```

Expected: commit succeeds with the renderer change and tests.

---

### Task 5: Verify mixed exploration behavior and full shell package stability

**Files:**
- Modify: `internal/ui/shell/model_output_test.go`
- Test: `internal/ui/shell/model_output_test.go`

- [ ] **Step 1: Add a mixed exploration regression test**

Add this test near the other runtime-model transcript tests in `internal/ui/shell/model_output_test.go`:

```go
func TestRuntimeModelMixedExplorationItemsEachKeepTheirOwnPreview(t *testing.T) {
	model := NewRuntimeModel()
	model = model.ApplyEvent(runtimeevents.StepBegin{Number: 1})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "read-1",
		Name:      "read_file",
		Subtitle:  "Read internal/ui/shell/model.go",
		Arguments: `{"path":"internal/ui/shell/model.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "read-1",
		ToolName:      "read_file",
		Output:        "package shell",
		DisplayOutput: "package shell",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "glob-1",
		Name:      "glob",
		Subtitle:  "Matched internal/ui/shell/*.go",
		Arguments: `{"pattern":"internal/ui/shell/*.go"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "glob-1",
		ToolName:      "glob",
		Output:        "internal/ui/shell/model.go\ninternal/ui/shell/model_output.go",
		DisplayOutput: "internal/ui/shell/model.go\ninternal/ui/shell/model_output.go",
	})
	model = model.ApplyEvent(runtimeevents.ToolCall{
		ID:        "grep-1",
		Name:      "grep",
		Subtitle:  `Searched internal/ui/shell for "approval"`,
		Arguments: `{"path":"internal/ui/shell","pattern":"approval"}`,
	})
	model = model.ApplyEvent(runtimeevents.ToolResult{
		ToolCallID:    "grep-1",
		ToolName:      "grep",
		Output:        "internal/ui/shell/model.go:1:approvalRequestMsg",
		DisplayOutput: "internal/ui/shell/model.go:1:approvalRequestMsg",
	})

	blocks := model.ToBlocks()
	if len(blocks) != 1 {
		t.Fatalf("len(ToBlocks()) = %d, want 1", len(blocks))
	}
	if len(blocks[0].Activity.Items) != 3 {
		t.Fatalf("len(blocks[0].Activity.Items) = %d, want 3", len(blocks[0].Activity.Items))
	}
	for i, want := range []string{"package shell", "model_output.go", "approvalRequestMsg"} {
		if !strings.Contains(blocks[0].Activity.Items[i].Preview.Text, want) {
			t.Fatalf("items[%d].Preview.Text = %q, want to contain %q", i, blocks[0].Activity.Items[i].Preview.Text, want)
		}
	}
}
```

- [ ] **Step 2: Run the mixed test to verify it passes**

Run:

```bash
go test ./internal/ui/shell -run TestRuntimeModelMixedExplorationItemsEachKeepTheirOwnPreview -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the full shell package test suite**

Run:

```bash
go test ./internal/ui/shell/... -count=1
```

Expected: PASS for all shell package tests with no new failures.

- [ ] **Step 4: Commit the final regression coverage**

Run:

```bash
git add internal/ui/shell/model_output_test.go
git commit -m "test(shell): cover mixed grouped explore previews"
```

Expected: commit succeeds with final regression coverage.

---

## Spec Coverage Check

- **Grouped `Explored` cards remain grouped:** Task 3 keeps grouping unchanged and only changes where previews are stored.
- **Collapsed state shows list only:** Tasks 1 and 4 verify collapsed cards do not show preview bodies.
- **Expanded state shows each item preview in order:** Tasks 1 and 4 add direct rendering coverage for ordered previews.
- **Existing diff rendering preserved:** Task 4 leaves non-explored preview rendering on the existing `renderPreviewBody` path.
- **Legacy group-level preview fallback preserved:** Task 4 adds explicit fallback coverage.
- **Mixed grouped exploration tools supported:** Task 5 adds mixed `read/glob/grep` regression coverage.
- **No new keyboard behavior:** Task 4 keeps `ToggleExpand()` semantics intact and re-runs the latest-collapsible test.

## Placeholder Scan

- No `TODO`, `TBD`, or deferred implementation notes remain.
- Every code-changing step includes exact code.
- Every verification step includes an exact command and expected result.
- Every task includes exact file paths and commit commands.

## Type Consistency Check

- `ActivityItem.Preview` is introduced in Task 2 and used consistently in later tasks.
- Group-level legacy fallback remains `ActivityGroupBlock.Preview` throughout the plan.
- Rendering helpers consistently treat grouped exploration cards as `GroupKind == "explored"`.
- Test names, helper names, and field names match across tasks.
