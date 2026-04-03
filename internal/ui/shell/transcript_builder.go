package shell

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/wire"
)

const (
	defaultPreviewLineLimit = 8
	diffPreviewLineLimit    = 14
)

type activityRef struct {
	blockIdx int
	itemIdx  int
	toolName string
}

type transcriptBuilder struct {
	blocks              []TranscriptBlock
	currentAssistantIdx int
	currentActivityIdx  int
	toolRefs            map[string]activityRef
	approvalRefs        map[string]int
	nextID              int
	turnStartedAt       time.Time
	lastElapsedAt       time.Time
}

func newTranscriptBuilder() transcriptBuilder {
	return transcriptBuilder{
		currentAssistantIdx: -1,
		currentActivityIdx:  -1,
		toolRefs:            make(map[string]activityRef),
		approvalRefs:        make(map[string]int),
	}
}

func (b *transcriptBuilder) resetTurn(now time.Time) {
	b.blocks = nil
	b.currentAssistantIdx = -1
	b.currentActivityIdx = -1
	b.toolRefs = make(map[string]activityRef)
	b.approvalRefs = make(map[string]int)
	b.turnStartedAt = now
	b.lastElapsedAt = now
}

func (b *transcriptBuilder) snapshot() []TranscriptBlock {
	blocks := make([]TranscriptBlock, len(b.blocks))
	copy(blocks, b.blocks)
	for i := range blocks {
		if len(blocks[i].Activity.Items) > 0 {
			items := make([]ActivityItem, len(blocks[i].Activity.Items))
			copy(items, blocks[i].Activity.Items)
			blocks[i].Activity.Items = items
		}
	}
	return blocks
}

func (b *transcriptBuilder) appendBlock(block TranscriptBlock) int {
	if block.ID == "" {
		b.nextID++
		block.ID = fmt.Sprintf("block-%d", b.nextID)
	}
	if block.CreatedAt.IsZero() {
		block.CreatedAt = time.Now()
	}
	b.blocks = append(b.blocks, block)
	return len(b.blocks) - 1
}

func (b *transcriptBuilder) appendDividerIfNeeded() {
	if len(b.blocks) == 0 {
		return
	}
	if b.blocks[len(b.blocks)-1].Kind == BlockKindDivider {
		return
	}
	b.appendBlock(TranscriptBlock{Kind: BlockKindDivider})
}

func (b *transcriptBuilder) closeAssistant() {
	b.currentAssistantIdx = -1
}

func (b *transcriptBuilder) closeActivity() {
	b.currentActivityIdx = -1
}

func (b *transcriptBuilder) appendUser(text string) {
	b.closeAssistant()
	b.closeActivity()
	b.appendBlock(TranscriptBlock{
		Kind:     BlockKindUserPrompt,
		UserText: strings.TrimSpace(text),
	})
}

func (b *transcriptBuilder) appendSystem(text string) {
	b.closeAssistant()
	b.closeActivity()
	b.appendBlock(TranscriptBlock{
		Kind: BlockKindSystemNotice,
		Text: strings.TrimSpace(text),
	})
}

func (b *transcriptBuilder) appendError(text string) {
	b.closeAssistant()
	b.closeActivity()
	b.appendBlock(TranscriptBlock{
		Kind: BlockKindError,
		Text: strings.TrimSpace(text),
	})
}

func (b *transcriptBuilder) appendAssistantText(text string) {
	if text == "" {
		return
	}
	if b.currentActivityIdx >= 0 {
		b.closeActivity()
		b.appendDividerIfNeeded()
	}
	if b.currentAssistantIdx < 0 {
		b.currentAssistantIdx = b.appendBlock(TranscriptBlock{
			Kind:     BlockKindAssistantNote,
			NoteText: text,
		})
		return
	}
	b.blocks[b.currentAssistantIdx].NoteText += text
}

func (b *transcriptBuilder) ensureActivityGroup(spec activityGroupSpec) int {
	if spec.Reuse && b.currentActivityIdx >= 0 {
		current := &b.blocks[b.currentActivityIdx]
		if current.Kind == BlockKindActivityGroup && current.Activity.GroupKind == spec.GroupKind {
			b.closeAssistant()
			return b.currentActivityIdx
		}
	}

	b.closeAssistant()
	b.closeActivity()
	idx := b.appendBlock(TranscriptBlock{
		Kind: BlockKindActivityGroup,
		Activity: ActivityGroupBlock{
			GroupKind: spec.GroupKind,
			Title:     spec.Title,
			Accent:    spec.Accent,
		},
	})
	b.currentActivityIdx = idx
	return idx
}

func (b *transcriptBuilder) applyEvent(event runtimeevents.Event) {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		b.resetTurn(time.Now())
	case runtimeevents.TextPart:
		b.appendAssistantText(e.Text)
	case runtimeevents.ToolCall:
		b.applyToolCall(e)
	case runtimeevents.ToolCallPart:
		b.applyToolCallPart(e)
	case runtimeevents.ToolResult:
		b.applyToolResult(e)
	case runtimeevents.StepInterrupted:
		b.closeAssistant()
		b.closeActivity()
		b.appendBlock(TranscriptBlock{
			Kind: BlockKindSystemNotice,
			Text: "Interrupted",
		})
	}
}

func (b *transcriptBuilder) applyToolCall(call runtimeevents.ToolCall) {
	spec := classifyToolCall(call)
	if spec.NoteText != "" {
		b.appendAssistantText(spec.NoteText)
		return
	}
	if spec.GroupKind == "" {
		return
	}

	blockIdx := b.ensureActivityGroup(spec)
	if spec.ItemVerb != "" || spec.ItemText != "" {
		block := &b.blocks[blockIdx]
		block.Activity.Items = append(block.Activity.Items, ActivityItem{
			ToolCallID: call.ID,
			Verb:       spec.ItemVerb,
			Text:       spec.ItemText,
			Status:     ActivityItemRunning,
		})
		b.toolRefs[call.ID] = activityRef{blockIdx: blockIdx, itemIdx: len(block.Activity.Items) - 1, toolName: call.Name}
		return
	}

	b.toolRefs[call.ID] = activityRef{blockIdx: blockIdx, itemIdx: -1, toolName: call.Name}
}

func (b *transcriptBuilder) applyToolCallPart(part runtimeevents.ToolCallPart) {
	ref, ok := b.toolRefs[part.ToolCallID]
	if !ok {
		return
	}
	if ref.itemIdx >= 0 {
		item := &b.blocks[ref.blockIdx].Activity.Items[ref.itemIdx]
		item.Text += part.Delta
		return
	}
	if b.blocks[ref.blockIdx].Activity.GroupKind == "command" {
		b.blocks[ref.blockIdx].Activity.Title += part.Delta
	}
}

func (b *transcriptBuilder) applyToolResult(result runtimeevents.ToolResult) {
	ref, ok := b.toolRefs[result.ToolCallID]
	if !ok {
		if result.IsError && strings.TrimSpace(result.Output) != "" {
			b.appendError(result.Output)
		}
		return
	}

	toolName := ref.toolName
	if toolName == "" {
		toolName = result.ToolName
	}
	if toolName == "think" && !result.IsError {
		return
	}

	block := &b.blocks[ref.blockIdx]
	if ref.itemIdx >= 0 {
		if result.IsError {
			block.Activity.Items[ref.itemIdx].Status = ActivityItemFailed
		} else {
			block.Activity.Items[ref.itemIdx].Status = ActivityItemCompleted
		}
	}

	if title := summarizeActivityTitle(toolName, result.Output, result.DisplayOutput); title != "" {
		block.Activity.Title = title
	}

	previewText := normalizePreviewText(block.Activity.Title, toolResultPreviewOutput(toolName, result.Output, result.DisplayOutput))
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
}

func (b *transcriptBuilder) tick(now time.Time) bool {
	if b.turnStartedAt.IsZero() || len(b.blocks) == 0 {
		return false
	}
	updated := false
	for now.Sub(b.lastElapsedAt) >= time.Minute {
		b.appendBlock(TranscriptBlock{
			Kind: BlockKindElapsed,
			Text: formatWorkedFor(now.Sub(b.turnStartedAt)),
		})
		b.lastElapsedAt = b.lastElapsedAt.Add(time.Minute)
		updated = true
	}
	return updated
}

func (b *transcriptBuilder) upsertApproval(req *wire.ApprovalRequest, selection int) {
	if req == nil {
		return
	}
	if idx, ok := b.approvalRefs[req.ID]; ok {
		block := &b.blocks[idx]
		block.Approval.Selected = selection
		block.Approval.Action = strings.TrimSpace(req.Action)
		block.Approval.Description = strings.TrimSpace(req.Description)
		return
	}

	b.closeAssistant()
	b.closeActivity()
	idx := b.appendBlock(TranscriptBlock{
		Kind: BlockKindApproval,
		Approval: ApprovalBlock{
			RequestID:   req.ID,
			Action:      strings.TrimSpace(req.Action),
			Description: strings.TrimSpace(req.Description),
			Selected:    selection,
			Status:      ApprovalStatusPending,
		},
	})
	b.approvalRefs[req.ID] = idx
}

func (b *transcriptBuilder) updateApprovalSelection(id string, selection int) bool {
	idx, ok := b.approvalRefs[id]
	if !ok {
		return false
	}
	b.blocks[idx].Approval.Selected = selection
	return true
}

func (b *transcriptBuilder) resolveApproval(id string, resp wire.ApprovalResponse) bool {
	idx, ok := b.approvalRefs[id]
	if !ok {
		return false
	}
	switch resp {
	case wire.ApprovalApprove:
		b.blocks[idx].Approval.Status = ApprovalStatusApproved
	case wire.ApprovalApproveForSession:
		b.blocks[idx].Approval.Status = ApprovalStatusApprovedForSession
	default:
		b.blocks[idx].Approval.Status = ApprovalStatusRejected
	}
	return true
}

func buildTranscriptBlocksFromRecords(records []contextstore.TextRecord) []TranscriptBlock {
	builder := newTranscriptBuilder()
	for _, record := range records {
		content := strings.TrimSpace(record.Content)
		if record.Role == contextstore.RoleSystem && content == "session initialized" {
			continue
		}

		switch record.Role {
		case contextstore.RoleUser:
			if content != "" {
				builder.appendUser(content)
			}
		case contextstore.RoleAssistant:
			if content != "" {
				builder.appendAssistantText(content)
			}
			if strings.TrimSpace(record.ToolCallsJSON) == "" {
				continue
			}
			var calls []runtime.ToolCall
			if err := json.Unmarshal([]byte(record.ToolCallsJSON), &calls); err != nil {
				continue
			}
			for _, call := range calls {
				builder.applyToolCall(runtimeevents.ToolCall{
					ID:        call.ID,
					Name:      call.Name,
					Subtitle:  runtime.ToolCallSubtitle(call),
					Arguments: call.Arguments,
				})
			}
		case contextstore.RoleTool:
			builder.applyToolResult(runtimeevents.ToolResult{
				ToolCallID:    record.ToolCallID,
				Output:        record.Content,
				DisplayOutput: firstNonEmpty(strings.TrimSpace(record.DisplayContent), record.Content),
			})
		case contextstore.RoleSystem:
			if content != "" {
				builder.appendSystem(content)
			}
		}
	}

	return builder.snapshot()
}

type activityGroupSpec struct {
	GroupKind string
	Title     string
	ItemVerb  string
	ItemText  string
	Reuse     bool
	Accent    string
	NoteText  string
}

func classifyToolCall(call runtimeevents.ToolCall) activityGroupSpec {
	summary := strings.TrimSpace(firstNonEmpty(call.Subtitle, toolCallDisplaySummary(call.Name, call.Subtitle, call.Arguments)))
	switch call.Name {
	case "think":
		return activityGroupSpec{NoteText: parseThinkNote(call.Arguments, summary)}
	case "read_file":
		return activityGroupSpec{GroupKind: "explored", Title: "Explored", ItemVerb: "Read", ItemText: firstNonEmpty(readToolPath(call.Arguments), strings.TrimPrefix(summary, "Read ")), Reuse: true, Accent: "explore"}
	case "glob":
		return activityGroupSpec{GroupKind: "explored", Title: "Explored", ItemVerb: "Search", ItemText: firstNonEmpty(globPattern(call.Arguments), strings.TrimPrefix(summary, "Matched ")), Reuse: true, Accent: "explore"}
	case "grep":
		return activityGroupSpec{GroupKind: "explored", Title: "Explored", ItemVerb: "Search", ItemText: firstNonEmpty(grepSummary(call.Arguments), strings.TrimPrefix(summary, "Searched ")), Reuse: true, Accent: "explore"}
	case "search_web":
		return activityGroupSpec{GroupKind: "explored", Title: "Explored", ItemVerb: "Search", ItemText: firstNonEmpty(searchQuery(call.Arguments), summary), Reuse: true, Accent: "explore"}
	case "fetch_url":
		return activityGroupSpec{GroupKind: "explored", Title: "Explored", ItemVerb: "Read", ItemText: firstNonEmpty(fetchURL(call.Arguments), summary), Reuse: true, Accent: "explore"}
	case "bash":
		return activityGroupSpec{GroupKind: "command", Title: firstNonEmpty(summary, "Ran command"), Accent: "command"}
	case "write_file":
		return activityGroupSpec{GroupKind: "edit", Title: firstNonEmpty(strings.Replace(summary, "Wrote ", "Edited ", 1), "Edited file"), Accent: "edit"}
	case "replace_file":
		return activityGroupSpec{GroupKind: "edit", Title: firstNonEmpty(summary, "Updated file"), Accent: "edit"}
	case "patch_file":
		return activityGroupSpec{GroupKind: "edit", Title: firstNonEmpty(strings.Replace(summary, "Patched ", "Edited ", 1), "Edited file"), Accent: "edit"}
	case "set_todo_list":
		return activityGroupSpec{GroupKind: "planned", Title: "Planned", Accent: "plan"}
	case "agent":
		return activityGroupSpec{GroupKind: "delegated", Title: "Delegated", ItemVerb: "Run", ItemText: firstNonEmpty(strings.TrimPrefix(summary, "Ran "), summary), Accent: "delegate"}
	default:
		return activityGroupSpec{GroupKind: "activity", Title: firstNonEmpty(summary, call.Name), Accent: "activity"}
	}
}

func parseThinkNote(arguments string, fallback string) string {
	var args struct {
		Thought string `json:"thought"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err == nil {
		if thought := strings.TrimSpace(args.Thought); thought != "" {
			return thought
		}
	}
	return strings.TrimSpace(strings.TrimPrefix(fallback, "Thought: "))
}

func readToolPath(arguments string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.Path)
}

func globPattern(arguments string) string {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.Pattern)
}

func grepSummary(arguments string) string {
	var args struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	path := strings.TrimSpace(args.Path)
	pattern := strings.TrimSpace(args.Pattern)
	switch {
	case path != "" && pattern != "":
		return path + " for " + pattern
	case path != "":
		return path
	default:
		return pattern
	}
}

func searchQuery(arguments string) string {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.Query)
}

func fetchURL(arguments string) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.URL)
}

func summarizeActivityTitle(toolName, output, displayOutput string) string {
	summary := strings.TrimSpace(firstNonEmpty(displayOutput, output))
	if summary == "" {
		return ""
	}
	firstLine := summary
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	switch toolName {
	case "bash":
		if strings.HasPrefix(firstLine, "Ran ") {
			return strings.TrimSpace(firstLine)
		}
		return ""
	case "write_file", "replace_file", "patch_file":
		return strings.TrimSpace(firstLine)
	default:
		return ""
	}
}

func normalizePreviewText(title string, preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		return ""
	}
	lines := strings.Split(preview, "\n")
	if len(lines) > 1 && strings.TrimSpace(lines[0]) == strings.TrimSpace(title) {
		return strings.Join(lines[1:], "\n")
	}
	return preview
}

func classifyPreviewKind(toolName string, preview string) PreviewKind {
	switch toolName {
	case "replace_file", "patch_file":
		return PreviewKindDiff
	}
	lines := strings.Split(preview, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			return PreviewKindDiff
		}
	}
	return PreviewKindText
}

func previewDefaultLimit(kind PreviewKind) int {
	if kind == PreviewKindDiff {
		return diffPreviewLineLimit
	}
	return defaultPreviewLineLimit
}

func previewLineCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(strings.Split(strings.TrimRight(text, "\n"), "\n"))
}

func formatWorkedFor(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	totalSeconds := int(duration.Round(time.Second).Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("Worked for %dm %02ds", minutes, seconds)
}
