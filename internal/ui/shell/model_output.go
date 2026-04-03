package shell

import (
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/renderers"
	"fimi-cli/internal/ui/shell/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const defaultRenderWidth = 80
const messageLabelWidth = 6

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
		return renderUserPromptBlock(block.UserText, m.panelWidth())
	case BlockKindAssistantNote:
		return m.renderAssistantNoteBlock(block.NoteText)
	case BlockKindActivityGroup:
		return m.renderActivityGroupBlock(block)
	case BlockKindApproval:
		return renderInlineApprovalBlock(block.Approval)
	case BlockKindDivider:
		return styles.TranscriptDividerStyle.Width(maxInt(1, m.renderWidth())).Render(strings.Repeat("鈹€", maxInt(1, m.renderWidth()-1)))
	case BlockKindElapsed:
		return styles.ElapsedStyle.Render(block.Text)
	case BlockKindError:
		return styles.ErrorNoticeStyle.Width(m.panelWidth()).Render(block.Text)
	case BlockKindSystemNotice:
		return styles.SystemNoticeStyle.Width(m.panelWidth()).Render(block.Text)
	default:
		return block.Text
	}
}

func renderUserPromptBlock(text string, width int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return styles.UserBubbleStyle.Width(width).Render(text)
}

func (m OutputModel) renderAssistantNoteBlock(note string) string {
	note = renderers.SanitizeMarkdown(note)
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	if !looksLikeMarkdownPreview(note) {
		return styles.AssistantBubbleStyle.Width(m.panelWidth()).Render(note)
	}
	contentWidth := m.assistantContentWidth()
	renderer, err := renderers.NewMarkdownRenderer(contentWidth)
	if err != nil {
		return styles.AssistantBubbleStyle.Width(m.panelWidth()).Render(note)
	}
	body := renderer.Render(note)
	if strings.TrimSpace(body) == "" {
		body = note
	}
	return styles.AssistantBubbleStyle.Width(m.panelWidth()).Render(body)
}

func (m OutputModel) renderActivityGroupBlock(block TranscriptBlock) string {
	group := block.Activity
	lines := []string{renderActivityTitle(group.Title, group.Accent)}
	for _, item := range group.Items {
		lines = append(lines, renderActivityItem(item))
	}

	if isGroupedExplorationCard(group) {
		if m.expanded[block.ID] {
			if previews := m.renderGroupedExplorationPreviews(block.ID, group); len(previews) > 0 {
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

func renderActivityTitle(title string, accent string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Activity"
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		renderActivityBadge(accent),
		" ",
		styles.ActivityTitleStyle.Render(title),
	)
}

func renderActivityItem(item ActivityItem) string {
	verb := strings.TrimSpace(item.Verb)
	text := strings.TrimSpace(item.Text)
	if verb == "" && text == "" {
		return ""
	}

	line := strings.TrimSpace(strings.Join([]string{verb, text}, " "))
	prefix := "  - "
	style := styles.ActivityPendingStyle
	switch item.Status {
	case ActivityItemRunning:
		prefix = "  > "
		style = styles.ActivityRunningStyle
	case ActivityItemCompleted:
		prefix = "  - "
		style = styles.ActivityCompletedStyle
	case ActivityItemFailed:
		prefix = "  ! "
		style = styles.ActivityFailedStyle
	}
	return style.Render(prefix + line)
}

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

func (m OutputModel) renderGroupedExplorationPreviews(blockID string, group ActivityGroupBlock) []string {
	var sections []string
	for _, item := range group.Items {
		if strings.TrimSpace(item.Preview.Text) == "" {
			continue
		}
		label := activityItemLabel(item)
		if label != "" {
			sections = append(sections, styles.ActivityDetailStyle.Render("    "+label))
		}
		if item.Preview.Kind == PreviewKindMarkdown {
			if body := m.renderMarkdownPreviewBody(item.Preview.Text); body != "" {
				sections = append(sections, body)
			}
			continue
		}
		for _, line := range strings.Split(strings.TrimRight(item.Preview.Text, "\n"), "\n") {
			sections = append(sections, renderPreviewLine(item.Preview.Kind, line))
		}
	}
	if len(sections) > 0 {
		return sections
	}
	if body := m.renderPreviewBody(blockID, group.Title, PreviewBody{
		Text:        group.Preview.Text,
		Kind:        group.Preview.Kind,
		Collapsible: false,
	}); body != "" {
		return []string{body}
	}
	return nil
}

func (m OutputModel) renderPreviewBody(blockID string, title string, preview PreviewBody) string {
	preview.Text = strings.TrimSpace(preview.Text)
	if preview.Text == "" {
		return ""
	}
	expanded := m.expanded[blockID]
	if preview.Kind == PreviewKindDiff {
		if rendered, ok := renderEditDiffPreviewBodyWithWidth(title, preview.Text, expanded, m.activityPreviewWidth()); ok {
			return rendered
		}
	}
	if preview.Kind == PreviewKindMarkdown {
		return m.renderMarkdownPreviewBody(preview.Text)
	}
	limit := previewDefaultLimit(preview.Kind)
	hint := previewToggleHint(expanded)

	lines := strings.Split(strings.TrimRight(preview.Text, "\n"), "\n")
	hidden := 0
	if preview.Collapsible && !expanded && len(lines) > limit {
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
			rendered = append(rendered, renderPreviewFooter("    ", preview.Kind, hidden, suffix, hint))
		} else {
			rendered = append(rendered, renderPreviewFooter("    ", preview.Kind, 0, "", hint))
		}
	}
	return strings.Join(rendered, "\n")
}

func renderPreviewLine(kind PreviewKind, line string) string {
	line = ansi.Strip(line)
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

	lines := []string{renderApprovalHeader(title)}
	if block.Action != "" {
		lines = append(lines, styles.ActivityDetailStyle.Render("  "+block.Action))
	}
	if block.Description != "" {
		lines = append(lines, styles.ActivityPreviewStyle.Render("  "+block.Description))
	}

	if block.Status != ApprovalStatusPending {
		return styles.ApprovalCardStyle.Render(strings.Join(lines, "\n"))
	}

	options := []string{"Approve", "Approve for session", "Reject"}
	for i, option := range options {
		if i == block.Selected {
			lines = append(lines, styles.ApprovalSelectedStyle.Render("  > "+option))
			continue
		}
		lines = append(lines, styles.ApprovalOptionStyle.Render("  "+option))
	}
	return styles.ApprovalCardStyle.Render(strings.Join(lines, "\n"))
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

	lines := []string{renderApprovalHeader(strings.TrimPrefix(title, "- "))}
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
	return styles.ApprovalCardStyle.Render(strings.Join(lines, "\n"))
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
		previewText := normalizePreviewText(title, toolResultPreviewOutput(e.ToolName, e.Output, e.DisplayOutput))
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

func (m OutputModel) panelWidth() int {
	return panelWidthForRenderWidth(m.renderWidth())
}

func (m OutputModel) activityPreviewWidth() int {
	width := m.panelWidth() - styles.ActivityCardStyle.GetHorizontalFrameSize()
	if width < 1 {
		return 1
	}
	return width
}

func (m OutputModel) renderMarkdownPreviewBody(text string) string {
	text = renderers.SanitizeMarkdown(text)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	renderer, err := renderers.NewMarkdownRenderer(m.activityPreviewWidth())
	if err != nil {
		return styles.ActivityPreviewStyle.Render("    " + strings.TrimSpace(text))
	}
	body := renderer.Render(text)
	if strings.TrimSpace(body) == "" {
		body = strings.TrimSpace(text)
	}
	return lipgloss.NewStyle().PaddingLeft(4).Render(body)
}

func (m OutputModel) assistantContentWidth() int {
	width := m.panelWidth() - styles.AssistantBubbleStyle.GetHorizontalFrameSize()
	if width < 1 {
		return 1
	}
	return width
}

func activityCardStyle(accent string) lipgloss.Style {
	return styles.ActivityCardStyle.Copy().BorderForeground(activityAccentColor(accent))
}

func renderActivityBadge(accent string) string {
	style := styles.ActivityBadgeBaseStyle.Copy().
		Foreground(activityAccentColor(accent)).
		Underline(true)
	return style.Render(activityBadgeLabel(accent))
}

func activityBadgeLabel(accent string) string {
	switch strings.TrimSpace(accent) {
	case "explore":
		return "EXPLORED"
	case "command":
		return "COMMAND"
	case "edit":
		return "EDIT"
	case "plan":
		return "PLAN"
	case "delegate":
		return "AGENT"
	default:
		return "ACTION"
	}
}

func activityAccentColor(accent string) styles.Color {
	switch strings.TrimSpace(accent) {
	case "explore":
		return styles.ColorInfo
	case "command":
		return styles.ColorAccent
	case "edit":
		return styles.ColorSecondary
	case "plan":
		return styles.ColorPrimary
	case "delegate":
		return styles.ColorWarning
	default:
		return styles.ColorBorder
	}
}

func activityBadgeForeground(accent string) styles.Color {
	switch strings.TrimSpace(accent) {
	case "explore", "command", "plan", "delegate":
		return styles.ColorBlack
	default:
		return styles.ColorWhite
	}
}

func renderPreviewFooter(indent string, kind PreviewKind, hidden int, suffix string, hint string) string {
	parts := []string{indent}
	parts = append(parts, styles.ActivityPreviewLabelStyle.Render(previewKindLabel(kind)))
	if hidden > 0 || hint != "" {
		parts = append(parts, styles.HelpStyle.Render(" "))
	}
	if hidden > 0 {
		parts = append(parts, styles.PreviewFooterStyle.Render(fmt.Sprintf("+%d %s", hidden, suffix)))
		if hint != "" {
			parts = append(parts, styles.HelpStyle.Render(" "))
		}
	}
	if hint != "" {
		parts = append(parts, styles.PreviewHintStyle.Render(hint))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func previewKindLabel(kind PreviewKind) string {
	if kind == PreviewKindDiff {
		return "Diff"
	}
	return "Preview"
}

func renderLabeledMessage(label string, labelStyle lipgloss.Style, body string) string {
	labelBox := labelStyle.Copy().
		Width(messageLabelWidth).
		Align(lipgloss.Right).
		Render(label)
	return lipgloss.JoinHorizontal(lipgloss.Top, labelBox, " ", body)
}

func messageBodyWidth(totalWidth int) int {
	width := totalWidth - messageLabelWidth - 1
	if width < 12 {
		return 12
	}
	return width
}

func panelWidthForRenderWidth(totalWidth int) int {
	width := totalWidth - 2
	if width < 20 {
		return 20
	}
	return width
}

func transcriptBodyIndent() string {
	return strings.Repeat(" ", messageLabelWidth+1)
}

func renderTranscriptBodyBlock(block string) string {
	return lipgloss.NewStyle().
		MarginLeft(messageLabelWidth + 1).
		Render(block)
}

func renderApprovalHeader(title string) string {
	badge := styles.ActivityBadgeBaseStyle.Copy().
		Foreground(styles.ColorWarning).
		Underline(true).
		Render("APPROVAL")
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		badge,
		" ",
		styles.ApprovalTitleStyle.Render(title),
	)
}

// wrapString wraps rendered terminal content without splitting ANSI escape
// sequences. Use wcwidth semantics so CJK text keeps the correct display width.
func wrapString(text string, width int) string {
	if width <= 0 {
		return text
	}
	return ansi.WrapWc(text, width, "")
}
