package shell

import (
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const defaultRenderWidth = 80

type LineType int

const (
	LineTypeUser LineType = iota
	LineTypeAssistant
	LineTypeToolCall
	LineTypeToolResult
	LineTypeSystem
	LineTypeError
)

type TranscriptLine struct {
	Type    LineType
	Content string
	Time    time.Time
}

type OutputModel struct {
	blocks       []TranscriptBlock
	printedCount int
	pending      []TranscriptBlock

	width          int
	height         int
	viewportHeight int
	atBottom       bool
	scrollOffset   int

	expanded map[string]bool
	nextID   int
}

type indexedTranscriptBlock struct {
	idx   int
	block TranscriptBlock
}

func NewOutputModel() OutputModel {
	return OutputModel{
		blocks:   make([]TranscriptBlock, 0),
		pending:  make([]TranscriptBlock, 0),
		atBottom: true,
		expanded: make(map[string]bool),
	}
}

func (m OutputModel) AppendLine(line TranscriptLine) OutputModel {
	block := transcriptBlockFromLine(line)
	return m.AppendBlock(block)
}

func (m OutputModel) AppendBlock(block TranscriptBlock) OutputModel {
	if block.ID == "" {
		m.nextID++
		block.ID = fmt.Sprintf("output-block-%d", m.nextID)
	}
	if block.CreatedAt.IsZero() {
		block.CreatedAt = time.Now()
	}
	m.blocks = append(m.blocks, block)
	if m.atBottom {
		m.scrollOffset = 0
	}
	return m
}

func transcriptBlockFromLine(line TranscriptLine) TranscriptBlock {
	switch line.Type {
	case LineTypeUser:
		return TranscriptBlock{Kind: BlockKindUserPrompt, UserText: line.Content}
	case LineTypeAssistant:
		return TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: line.Content}
	case LineTypeSystem:
		return TranscriptBlock{Kind: BlockKindSystemNotice, Text: line.Content}
	case LineTypeError:
		return TranscriptBlock{Kind: BlockKindError, Text: line.Content}
	case LineTypeToolCall:
		return TranscriptBlock{
			Kind: BlockKindActivityGroup,
			Activity: ActivityGroupBlock{
				GroupKind: "activity",
				Title:     line.Content,
			},
		}
	case LineTypeToolResult:
		return TranscriptBlock{
			Kind: BlockKindActivityGroup,
			Activity: ActivityGroupBlock{
				GroupKind: "activity",
				Title:     "Result",
				Preview: PreviewBody{
					Text:        line.Content,
					Kind:        classifyPreviewKind("", line.Content),
					Collapsible: previewLineCount(line.Content) > previewDefaultLimit(classifyPreviewKind("", line.Content)),
				},
				Collapsible: previewLineCount(line.Content) > previewDefaultLimit(classifyPreviewKind("", line.Content)),
			},
		}
	default:
		return TranscriptBlock{Kind: BlockKindSystemNotice, Text: line.Content}
	}
}

func transcriptLineModelsFromRecords(records []contextstore.TextRecord) []TranscriptBlock {
	return buildTranscriptBlocksFromRecords(records)
}

func (m OutputModel) SetPending(blocks []TranscriptBlock) OutputModel {
	m.pending = cloneTranscriptBlocks(blocks)
	if m.atBottom {
		m.scrollOffset = 0
	}
	return m
}

func (m OutputModel) FlushPending() OutputModel {
	m.blocks = append(m.blocks, cloneTranscriptBlocks(m.pending)...)
	m.pending = nil
	if m.atBottom {
		m.scrollOffset = 0
	}
	return m
}

func (m OutputModel) Clear() OutputModel {
	m.blocks = nil
	m.printedCount = 0
	m.pending = nil
	m.scrollOffset = 0
	m.atBottom = true
	m.expanded = make(map[string]bool)
	return m
}

func (m OutputModel) Update(msg tea.Msg, width, height int) (OutputModel, tea.Cmd) {
	m.width = width
	m.height = height
	switch msg := msg.(type) {
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollUp(3)
		case tea.MouseButtonWheelDown:
			m.scrollDown(3)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "pgup":
			m.scrollUp(m.visibleHeight())
		case "pgdown":
			m.scrollDown(m.visibleHeight())
		case "home":
			m.scrollToTop()
		case "end":
			m.scrollToBottom()
		}
	}
	return m, nil
}

func (m OutputModel) View() string {
	rows := m.renderedRows()
	if len(rows) == 0 {
		return ""
	}
	startIdx, endIdx := m.visibleRange(len(rows), m.visibleHeight())
	return strings.Join(rows[startIdx:endIdx], "\n")
}

func (m OutputModel) PendingView() string {
	if len(m.pending) == 0 {
		return ""
	}
	pendingOnly := m
	pendingOnly.blocks = nil
	pendingOnly.printedCount = 0
	return pendingOnly.View()
}

func (m OutputModel) InteractiveView() string {
	selection, anchorTop := m.interactiveSelection()
	if len(selection) == 0 {
		return ""
	}
	return m.renderIndexedSelection(selection, anchorTop)
}

func (m OutputModel) renderBlock(block TranscriptBlock) string {
	if block.Kind == BlockKindDivider {
		return styles.TranscriptDividerStyle.Width(maxInt(1, m.renderWidth())).Render(strings.Repeat("-", maxInt(1, m.renderWidth()-1)))
	}

	switch block.Kind {
	case BlockKindUserPrompt:
		return styles.UserBubbleStyle.Width(maxInt(1, m.renderWidth()-2)).Render(block.UserText)
	case BlockKindAssistantNote:
		return renderAssistantNoteBlock(block.NoteText)
	case BlockKindActivityGroup:
		return m.renderActivityGroupBlock(block)
	case BlockKindApproval:
		return renderInlineApprovalBlock(block.Approval)
	case BlockKindDivider:
		return styles.TranscriptDividerStyle.Width(maxInt(1, m.renderWidth())).Render(strings.Repeat("鈹€", maxInt(1, m.renderWidth()-1)))
	case BlockKindElapsed:
		return styles.ElapsedStyle.Render(block.Text)
	case BlockKindError:
		return styles.ErrorStyle.Render(block.Text)
	case BlockKindSystemNotice:
		return styles.SystemStyle.Render(block.Text)
	default:
		return block.Text
	}
}

func renderAssistantNoteBlock(note string) string {
	paragraphs := splitParagraphs(note)
	if len(paragraphs) == 0 {
		return ""
	}
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		lines = append(lines, styles.AssistantBulletStyle.Render(paragraph))
	}
	return strings.Join(lines, "\n\n")
}

func splitParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "\n\n")
	paragraphs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paragraphs = append(paragraphs, strings.ReplaceAll(part, "\n", " "))
		}
	}
	if len(paragraphs) == 0 && strings.TrimSpace(text) != "" {
		return []string{strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))}
	}
	return paragraphs
}

func (m OutputModel) renderActivityGroupBlock(block TranscriptBlock) string {
	group := block.Activity
	lines := []string{renderActivityTitle(group.Title, group.Accent)}
	for _, item := range group.Items {
		lines = append(lines, renderActivityItem(item))
	}
	if preview := m.renderPreviewBody(block.ID, group.Title, group.Preview); preview != "" {
		lines = append(lines, preview)
	}
	return strings.Join(lines, "\n")
}

func renderActivityTitle(title string, accent string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Activity"
	}
	return styles.ActivityTitleStyle.Render("- " + title)
}

func renderActivityItem(item ActivityItem) string {
	verb := strings.TrimSpace(item.Verb)
	text := strings.TrimSpace(item.Text)
	if verb == "" && text == "" {
		return ""
	}
	if text == "" {
		return styles.ActivityDetailStyle.Render("  - " + verb)
	}
	return styles.ActivityDetailStyle.Render("  - " + verb + " " + text)
}

func (m OutputModel) renderPreviewBody(blockID string, title string, preview PreviewBody) string {
	preview.Text = strings.TrimSpace(preview.Text)
	if preview.Text == "" {
		return ""
	}
	expanded := m.expanded[blockID]
	if preview.Kind == PreviewKindDiff {
		if rendered, ok := renderEditDiffPreviewBody(title, preview.Text, expanded); ok {
			return rendered
		}
	}
	limit := previewDefaultLimit(preview.Kind)
	hint := "Ctrl+O to expand"
	if expanded {
		limit = expandedPreviewLineLimit
		hint = "Ctrl+O to collapse"
	}

	lines := strings.Split(strings.TrimRight(preview.Text, "\n"), "\n")
	hidden := 0
	if preview.Collapsible && len(lines) > limit {
		hidden = len(lines) - limit
		lines = lines[:limit]
	}

	rendered := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		rendered = append(rendered, renderPreviewLine(preview.Kind, line))
	}
	if preview.Collapsible {
		if hidden > 0 {
			suffix := "lines"
			if preview.Kind == PreviewKindDiff {
				suffix = "diff lines"
			}
			rendered = append(rendered, styles.HelpStyle.Render(fmt.Sprintf("    ... +%d %s (%s)", hidden, suffix, hint)))
		} else {
			rendered = append(rendered, styles.HelpStyle.Render("    ("+hint+")"))
		}
	}
	return strings.Join(rendered, "\n")
}

func renderPreviewLine(kind PreviewKind, line string) string {
	prefix := "    "
	switch {
	case kind == PreviewKindDiff && strings.HasPrefix(line, "@@"):
		return styles.ToolDiffHunkStyle.Render(prefix + line)
	case kind == PreviewKindDiff && strings.HasPrefix(line, "+"):
		return styles.ToolDiffAddedStyle.Render(prefix + line)
	case kind == PreviewKindDiff && strings.HasPrefix(line, "-"):
		return styles.ToolDiffRemovedStyle.Render(prefix + line)
	case kind == PreviewKindDiff:
		return styles.ToolDiffContextStyle.Render(prefix + line)
	default:
		return styles.ActivityPreviewStyle.Render(prefix + line)
	}
}

func renderInlineApprovalBlock(block ApprovalBlock) string {
	title := "Approval required"
	switch block.Status {
	case ApprovalStatusApproved:
		title = "Approved"
	case ApprovalStatusApprovedForSession:
		title = "Approved for session"
	case ApprovalStatusRejected:
		title = "Rejected"
	}

	lines := []string{styles.ApprovalTitleStyle.Render("- " + title)}
	if block.Action != "" {
		lines = append(lines, styles.ActivityDetailStyle.Render("  "+block.Action))
	}
	if block.Description != "" {
		lines = append(lines, styles.ActivityPreviewStyle.Render("  "+block.Description))
	}

	if block.Status != ApprovalStatusPending {
		return strings.Join(lines, "\n")
	}

	options := []string{"Approve", "Approve for session", "Reject"}
	for i, option := range options {
		if i == block.Selected {
			lines = append(lines, styles.ApprovalSelectedStyle.Render("  > "+option))
			continue
		}
		lines = append(lines, styles.ApprovalOptionStyle.Render("  "+option))
	}
	return strings.Join(lines, "\n")
}

func renderApprovalBlock(block ApprovalBlock) string {
	title := "- Approval required"
	switch block.Status {
	case ApprovalStatusApproved:
		title = "- Approved"
	case ApprovalStatusApprovedForSession:
		title = "- Approved for session"
	case ApprovalStatusRejected:
		title = "- Rejected"
	}

	lines := []string{styles.ApprovalTitleStyle.Render(title)}
	if block.Action != "" {
		lines = append(lines, styles.ActivityDetailStyle.Render("  "+block.Action))
	}
	if block.Description != "" {
		lines = append(lines, styles.ActivityPreviewStyle.Render("  "+block.Description))
	}

	options := []string{"Approve", "Approve for session", "Reject"}
	for i, option := range options {
		label := option
		if block.Status == ApprovalStatusPending && i == block.Selected {
			label = "> " + label
			lines = append(lines, styles.ApprovalSelectedStyle.Render("  "+label))
			continue
		}
		lines = append(lines, styles.ApprovalOptionStyle.Render("  "+label))
	}
	return strings.Join(lines, "\n")
}

func (m OutputModel) visibleHeight() int {
	if m.viewportHeight > 0 {
		return m.viewportHeight
	}
	availableHeight := m.height - 6
	if availableHeight < 1 {
		availableHeight = 1
	}
	return availableHeight
}

func (m OutputModel) WithViewportHeight(height int) OutputModel {
	if height < 1 {
		height = 1
	}
	m.viewportHeight = height
	return m
}

func (m OutputModel) totalLines() int {
	return len(m.renderedRows())
}

func (m *OutputModel) scrollUp(lines int) {
	if lines <= 0 {
		return
	}
	maxOffset := m.maxScrollOffset()
	if maxOffset <= 0 {
		m.scrollOffset = 0
		m.atBottom = true
		return
	}
	m.scrollOffset += lines
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	m.atBottom = m.scrollOffset == 0
}

func (m *OutputModel) scrollDown(lines int) {
	if lines <= 0 {
		return
	}
	m.scrollOffset -= lines
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	m.atBottom = m.scrollOffset == 0
}

func (m *OutputModel) scrollToTop() {
	m.scrollOffset = m.maxScrollOffset()
	m.atBottom = m.scrollOffset == 0
}

func (m *OutputModel) scrollToBottom() {
	m.scrollOffset = 0
	m.atBottom = true
}

func (m OutputModel) maxScrollOffset() int {
	total := m.totalLines()
	visible := m.visibleHeight()
	if total <= visible {
		return 0
	}
	return total - visible
}

func (m OutputModel) visibleRange(totalLines int, visibleHeight int) (int, int) {
	if totalLines <= visibleHeight {
		return 0, totalLines
	}
	maxOffset := totalLines - visibleHeight
	offset := m.scrollOffset
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	startIdx := totalLines - visibleHeight - offset
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + visibleHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}
	return startIdx, endIdx
}

func (m OutputModel) renderedRows() []string {
	allBlocks := m.allBlocks()
	if len(allBlocks) == 0 {
		return nil
	}
	rows := make([]string, 0, len(allBlocks))
	for _, block := range allBlocks {
		rendered := m.renderBlock(block)
		rows = append(rows, strings.Split(wrapString(rendered, m.renderWidth()), "\n")...)
	}
	return rows
}

func (m OutputModel) allBlocks() []TranscriptBlock {
	allBlocks := make([]TranscriptBlock, 0, len(m.blocks)+len(m.pending))
	allBlocks = append(allBlocks, m.blocks...)
	allBlocks = append(allBlocks, m.pending...)
	return allBlocks
}

func (m OutputModel) RenderUnprintedLines() []string {
	end := m.stablePrintedTarget()
	if m.printedCount >= end {
		return nil
	}
	rendered := make([]string, 0, end-m.printedCount)
	for idx := m.printedCount; idx < end; idx++ {
		rendered = append(rendered, m.renderBlock(m.blocks[idx]))
	}
	return rendered
}

func (m OutputModel) RenderUnprintedCommitted() []string {
	if m.printedCount >= len(m.blocks) {
		return nil
	}
	rendered := make([]string, 0, len(m.blocks)-m.printedCount)
	for idx := m.printedCount; idx < len(m.blocks); idx++ {
		rendered = append(rendered, m.renderBlock(m.blocks[idx]))
	}
	return rendered
}

func (m OutputModel) stablePrintedTarget() int {
	return m.interactiveTailStart()
}

func (m OutputModel) interactiveSelection() ([]indexedTranscriptBlock, bool) {
	tailStart := m.interactiveTailStart()
	selection := make([]indexedTranscriptBlock, 0, len(m.blocks)-tailStart+len(m.pending))
	for idx := tailStart; idx < len(m.blocks); idx++ {
		selection = append(selection, indexedTranscriptBlock{idx: idx, block: m.blocks[idx]})
	}
	base := len(m.blocks)
	for i, block := range m.pending {
		selection = append(selection, indexedTranscriptBlock{idx: base + i, block: block})
	}
	if len(selection) == 0 {
		return nil, false
	}
	return selection, false
}

func (m OutputModel) interactiveTailStart() int {
	if m.printedCount < 0 {
		return 0
	}
	if m.printedCount > len(m.blocks) {
		return len(m.blocks)
	}
	for i := len(m.blocks) - 1; i >= m.printedCount; i-- {
		if m.blocks[i].Kind == BlockKindUserPrompt {
			return i
		}
	}
	return m.printedCount
}

func (m OutputModel) renderIndexedSelection(selection []indexedTranscriptBlock, anchorTop bool) string {
	rows := make([]string, 0, len(selection))
	for _, item := range selection {
		rows = append(rows, strings.Split(wrapString(m.renderBlock(item.block), m.renderWidth()), "\n")...)
	}
	if len(rows) == 0 {
		return ""
	}
	visible := m.visibleHeight()
	start := 0
	end := len(rows)
	if len(rows) > visible {
		if anchorTop {
			end = visible
		} else {
			start = len(rows) - visible
		}
	}
	return strings.Join(rows[start:end], "\n")
}

func (m OutputModel) MarkPrinted() OutputModel {
	return m.MarkPrintedUntil(len(m.blocks))
}

func (m OutputModel) MarkPrintedUntil(count int) OutputModel {
	if count < 0 {
		count = 0
	}
	if count > len(m.blocks) {
		count = len(m.blocks)
	}
	m.printedCount = count
	return m
}

func (m OutputModel) AppendCommittedRuntimeEvent(event runtimeevents.Event) OutputModel {
	switch e := event.(type) {
	case runtimeevents.TextPart:
		if len(m.blocks) > 0 && m.blocks[len(m.blocks)-1].Kind == BlockKindAssistantNote {
			m.blocks[len(m.blocks)-1].NoteText += e.Text
			return m
		}
		return m.AppendBlock(TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: e.Text})
	case runtimeevents.ToolCall:
		spec := classifyToolCall(e)
		if spec.NoteText != "" {
			return m.AppendBlock(TranscriptBlock{Kind: BlockKindAssistantNote, NoteText: spec.NoteText})
		}
		return m.AppendBlock(TranscriptBlock{
			Kind: BlockKindActivityGroup,
			Activity: ActivityGroupBlock{
				GroupKind: spec.GroupKind,
				Title:     spec.Title,
				Items:     committedActivityItems(spec, e.ID),
				Accent:    spec.Accent,
			},
		})
	case runtimeevents.ToolResult:
		title := summarizeActivityTitle(e.ToolName, e.Output, e.DisplayOutput)
		previewText := normalizePreviewText(title, toolResultDisplayOutput(e.Output, e.DisplayOutput))
		return m.AppendBlock(TranscriptBlock{
			Kind: BlockKindActivityGroup,
			Activity: ActivityGroupBlock{
				GroupKind: firstNonEmpty(e.ToolName, "activity"),
				Title:     firstNonEmpty(title, e.ToolName),
				Preview: PreviewBody{
					Text:        previewText,
					Kind:        classifyPreviewKind(e.ToolName, previewText),
					Collapsible: previewLineCount(previewText) > previewDefaultLimit(classifyPreviewKind(e.ToolName, previewText)),
				},
				Collapsible: previewLineCount(previewText) > previewDefaultLimit(classifyPreviewKind(e.ToolName, previewText)),
			},
		})
	case runtimeevents.StepInterrupted:
		return m.AppendBlock(TranscriptBlock{Kind: BlockKindSystemNotice, Text: "Interrupted"})
	default:
		return m
	}
}

func committedActivityItems(spec activityGroupSpec, toolCallID string) []ActivityItem {
	if spec.ItemVerb == "" && spec.ItemText == "" {
		return nil
	}
	return []ActivityItem{{
		ToolCallID: toolCallID,
		Verb:       spec.ItemVerb,
		Text:       spec.ItemText,
		Status:     ActivityItemCompleted,
	}}
}

func (m OutputModel) ToggleExpand() (OutputModel, bool) {
	allBlocks := m.allBlocks()
	for i := len(allBlocks) - 1; i >= 0; i-- {
		block := allBlocks[i]
		if !block.IsCollapsible() {
			continue
		}
		m.expanded[block.ID] = !m.expanded[block.ID]
		return m, true
	}
	return m, false
}

func (m OutputModel) HasExpandedResults() bool {
	for _, expanded := range m.expanded {
		if expanded {
			return true
		}
	}
	return false
}

func cloneTranscriptBlocks(blocks []TranscriptBlock) []TranscriptBlock {
	cloned := make([]TranscriptBlock, len(blocks))
	copy(cloned, blocks)
	for i := range cloned {
		if len(cloned[i].Activity.Items) > 0 {
			items := make([]ActivityItem, len(cloned[i].Activity.Items))
			copy(items, cloned[i].Activity.Items)
			cloned[i].Activity.Items = items
		}
	}
	return cloned
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m OutputModel) renderWidth() int {
	if m.width <= 1 {
		return defaultRenderWidth
	}
	return m.width
}

// wrapString wraps rendered terminal content without splitting ANSI escape
// sequences. Use wcwidth semantics so CJK text keeps the correct display width.
func wrapString(text string, width int) string {
	if width <= 0 {
		return text
	}
	return ansi.WrapWc(text, width, "")
}
