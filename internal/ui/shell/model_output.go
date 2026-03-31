package shell

import (
	"fmt"
	"strings"
	"time"

	"encoding/json"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/bubbletea"
	"github.com/muesli/reflow/wrap"
)

// LineType 表示 transcript 行的类型。
type LineType int

const (
	// LineTypeUser 用户消息
	LineTypeUser LineType = iota
	// LineTypeAssistant 助手消息
	LineTypeAssistant
	// LineTypeToolCall 工具调用
	LineTypeToolCall
	// LineTypeToolResult 工具结果
	LineTypeToolResult
	// LineTypeSystem 系统消息
	LineTypeSystem
	// LineTypeError 错误消息
	LineTypeError
)

// TranscriptLine 表示 transcript 中的一行。
type TranscriptLine struct {
	Type    LineType
	Content string
	Time    time.Time
}

// 折叠阈值：超过此行数的结果将被折叠
const foldThreshold = 10

// OutputModel 管理可滚动的 transcript 显示。
type OutputModel struct {
	// 已完成的行
	lines []TranscriptLine
	// 已打印到终端 scrollback 的已完成行数量
	printedCount int
	// 正在更新的实时内容（流式输出）
	pending []TranscriptLine
	// 视口尺寸
	width          int
	height         int
	viewportHeight int
	// 是否自动滚动到底部
	atBottom bool
	// 相对底部的滚动偏移量（按行）
	scrollOffset int
	// 已展开的 ToolResult 行索引（key 为 lines 中的索引）
	expanded map[int]bool
}

type indexedTranscriptLine struct {
	idx  int
	line TranscriptLine
}

// NewOutputModel 创建一个新的输出模型。
func NewOutputModel() OutputModel {
	return OutputModel{
		lines:    make([]TranscriptLine, 0),
		pending:  make([]TranscriptLine, 0),
		atBottom: true,
		expanded: make(map[int]bool),
	}
}

// AppendLine 添加一行到 transcript。
func (m OutputModel) AppendLine(line TranscriptLine) OutputModel {
	line.Time = time.Now()
	m.lines = append(m.lines, line)
	if m.atBottom {
		m.scrollOffset = 0
	}
	return m
}

func transcriptLineModelsFromRecords(records []contextstore.TextRecord) []TranscriptLine {
	lines := make([]TranscriptLine, 0, len(records))
	for _, record := range records {
		content := strings.TrimSpace(record.Content)
		if record.Role == contextstore.RoleSystem && content == "session initialized" {
			continue
		}

		switch record.Role {
		case contextstore.RoleUser:
			if content == "" {
				continue
			}
			lines = append(lines, TranscriptLine{
				Type:    LineTypeUser,
				Content: content,
			})
		case contextstore.RoleAssistant:
			if content != "" {
				lines = append(lines, TranscriptLine{
					Type:    LineTypeAssistant,
					Content: content,
				})
			}
			for _, summary := range storedToolCallSummaries(record.ToolCallsJSON) {
				lines = append(lines, TranscriptLine{
					Type:    LineTypeToolCall,
					Content: summary,
				})
			}
		case contextstore.RoleTool:
			if content == "" {
				continue
			}
			lines = append(lines, TranscriptLine{
				Type:    LineTypeToolResult,
				Content: content,
			})
		}
	}

	return lines
}

func storedToolCallSummaries(encoded string) []string {
	if strings.TrimSpace(encoded) == "" {
		return nil
	}

	var calls []struct {
		Name      string
		Arguments string
	}
	if err := json.Unmarshal([]byte(encoded), &calls); err != nil {
		return nil
	}

	summaries := make([]string, 0, len(calls))
	for _, call := range calls {
		summary := strings.TrimSpace(runtime.ToolCallSubtitle(runtime.ToolCall{
			Name:      call.Name,
			Arguments: call.Arguments,
		}))
		if summary == "" {
			summary = strings.TrimSpace(toolCallDisplaySummary(call.Name, "", call.Arguments))
		}
		if summary == "" {
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries
}

// SetPending 用最新快照替换实时内容。
func (m OutputModel) SetPending(lines []TranscriptLine) OutputModel {
	m.pending = append([]TranscriptLine(nil), lines...)
	if m.atBottom {
		m.scrollOffset = 0
	}
	return m
}

// FlushPending 将实时内容刷新到已完成的行。
func (m OutputModel) FlushPending() OutputModel {
	m.lines = append(m.lines, m.pending...)
	m.pending = nil
	if m.atBottom {
		m.scrollOffset = 0
	}
	return m
}

// Clear 清空 transcript。
func (m OutputModel) Clear() OutputModel {
	m.lines = nil
	m.printedCount = 0
	m.pending = nil
	m.scrollOffset = 0
	m.atBottom = true
	m.expanded = make(map[int]bool)
	return m
}

// Update 处理消息并更新状态。
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

// View 渲染 transcript 视图。
func (m OutputModel) View() string {
	rows := m.renderedRows()

	if len(rows) == 0 {
		return ""
	}

	startIdx, endIdx := m.visibleRange(len(rows), m.visibleHeight())

	var b strings.Builder
	for i := startIdx; i < endIdx; i++ {
		if i > startIdx {
			b.WriteByte('\n')
		}
		b.WriteString(rows[i])
	}

	return b.String()
}

func (m OutputModel) PendingView() string {
	if len(m.pending) == 0 {
		return ""
	}
	pendingOnly := m
	pendingOnly.lines = nil
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

// renderLine 渲染单行。
// idx 参数用于查找该行的折叠状态（仅对 ToolResult 有效）。
func (m OutputModel) renderLine(line TranscriptLine, idx int) string {
	switch line.Type {
	case LineTypeUser:
		return styles.UserStyle.Render(line.Content)
	case LineTypeAssistant:
		return line.Content
	case LineTypeToolCall:
		return styles.ToolNameStyle.Render("● " + line.Content)
	case LineTypeToolResult:
		return m.renderToolResult(line.Content, idx)
	case LineTypeSystem:
		return styles.SystemStyle.Render(line.Content)
	case LineTypeError:
		return styles.ErrorStyle.Render(line.Content)
	default:
		return line.Content
	}
}

// renderToolResult 渲染工具结果，默认隐藏正文，展开后才显示完整内容。
func (m OutputModel) renderToolResult(content string, idx int) string {
	if m.expanded[idx] {
		return styles.SystemStyle.Render(m.expandedToolResultContent(content))
	}

	// Take only the first line for the preview
	preview := strings.TrimSpace(content)
	if preview == "" {
		preview = "No output"
	}
	if nl := strings.IndexByte(preview, '\n'); nl >= 0 {
		preview = preview[:nl]
	}
	if len(preview) > 80 {
		preview = preview[:77] + "..."
	}

	return styles.HelpStyle.Render("  ⎿  " + preview + "  (Ctrl+O to expand)")
}

func (m OutputModel) expandedToolResultContent(content string) string {
	trimmed := strings.TrimRight(content, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return "  ⎿  No output\n     (Ctrl+O to collapse)"
	}

	lines := strings.Split(trimmed, "\n")
	hidden := 0
	if len(lines) > foldThreshold {
		hidden = len(lines) - foldThreshold
		lines = lines[:foldThreshold]
	}

	formatted := make([]string, 0, len(lines)+1)
	for i, line := range lines {
		prefix := "     "
		if i == 0 {
			prefix = "  ⎿  "
		}
		formatted = append(formatted, prefix+line)
	}
	if hidden > 0 {
		formatted = append(formatted, fmt.Sprintf("     ... %d more lines hidden (Ctrl+O to collapse)", hidden))
	} else {
		formatted = append(formatted, "     (Ctrl+O to collapse)")
	}

	return strings.Join(formatted, "\n")
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
	allLines := m.allLines()
	if len(allLines) == 0 {
		return nil
	}

	renderWidth := m.renderWidth()
	rows := make([]string, 0, len(allLines))
	for idx, line := range allLines {
		rendered := m.renderLine(line, idx)
		wrapped := wrap.String(rendered, renderWidth)
		rows = append(rows, strings.Split(wrapped, "\n")...)
	}

	return rows
}

func (m OutputModel) allLines() []TranscriptLine {
	allLines := make([]TranscriptLine, 0, len(m.lines)+len(m.pending))
	allLines = append(allLines, m.lines...)
	allLines = append(allLines, m.pending...)
	return allLines
}

func (m OutputModel) RenderUnprintedLines() []string {
	tailStart := m.interactiveTailStart()
	if m.printedCount >= tailStart {
		return nil
	}

	rendered := make([]string, 0, tailStart-m.printedCount)
	for idx := m.printedCount; idx < tailStart; idx++ {
		rendered = append(rendered, m.renderLine(m.lines[idx], idx))
	}
	return rendered
}

func (m OutputModel) interactiveSelection() ([]indexedTranscriptLine, bool) {
	tailStart := m.interactiveTailStart()
	selection := make([]indexedTranscriptLine, 0, len(m.lines)-tailStart+len(m.pending))
	for idx := tailStart; idx < len(m.lines); idx++ {
		selection = append(selection, indexedTranscriptLine{idx: idx, line: m.lines[idx]})
	}

	base := len(m.lines)
	for i, line := range m.pending {
		selection = append(selection, indexedTranscriptLine{idx: base + i, line: line})
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
	if m.printedCount > len(m.lines) {
		return len(m.lines)
	}

	for i := len(m.lines) - 1; i >= m.printedCount; i-- {
		if m.lines[i].Type == LineTypeUser {
			return i
		}
	}

	return m.printedCount
}

func (m OutputModel) renderIndexedSelection(selection []indexedTranscriptLine, anchorTop bool) string {
	rows := make([]string, 0, len(selection))
	for _, item := range selection {
		rendered := m.renderLine(item.line, item.idx)
		rows = append(rows, strings.Split(wrap.String(rendered, m.renderWidth()), "\n")...)
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
	m.printedCount = len(m.lines)
	return m
}

func (m OutputModel) AppendCommittedRuntimeEvent(event runtimeevents.Event) OutputModel {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		return m.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: fmt.Sprintf("Step %d", e.Number),
		})
	case runtimeevents.StepInterrupted:
		return m.AppendLine(TranscriptLine{
			Type:    LineTypeSystem,
			Content: "Interrupted",
		})
	case runtimeevents.TextPart:
		return m.appendCommittedAssistantText(e.Text)
	case runtimeevents.ToolCall:
		return m.AppendLine(TranscriptLine{
			Type: LineTypeToolCall,
			Content: formatToolCallLine(ToolCallInfo{
				ID:     e.ID,
				Name:   e.Name,
				Status: ToolStatusRunning,
				Args:   toolCallDisplaySummary(e.Name, e.Subtitle, e.Arguments),
			}),
		})
	case runtimeevents.ToolCallPart:
		return m.appendCommittedToolCallDelta(e.Delta)
	case runtimeevents.ToolResult:
		lineType := LineTypeToolResult
		if e.IsError {
			lineType = LineTypeError
		}
		return m.AppendLine(TranscriptLine{
			Type:    lineType,
			Content: strings.TrimSpace(e.Output),
		})
	default:
		return m
	}
}

func (m OutputModel) appendCommittedAssistantText(delta string) OutputModel {
	if delta == "" {
		return m
	}
	if len(m.lines) > 0 && m.lines[len(m.lines)-1].Type == LineTypeAssistant {
		m.lines[len(m.lines)-1].Content += delta
		return m
	}
	return m.AppendLine(TranscriptLine{
		Type:    LineTypeAssistant,
		Content: delta,
	})
}

func (m OutputModel) appendCommittedToolCallDelta(delta string) OutputModel {
	if delta == "" {
		return m
	}
	for i := len(m.lines) - 1; i >= 0; i-- {
		if m.lines[i].Type == LineTypeToolCall {
			m.lines[i].Content += delta
			return m
		}
	}
	return m
}

func (m OutputModel) renderWidth() int {
	if m.width <= 1 {
		return 1
	}

	return m.width
}

// ToggleExpand 切换最后一个 ToolResult 行的折叠状态。
// 返回切换后的模型和是否找到了可切换的行。
func (m OutputModel) ToggleExpand() (OutputModel, bool) {
	allLines := m.allLines()
	lastToolResultIdx := -1
	for i := len(allLines) - 1; i >= 0; i-- {
		if allLines[i].Type == LineTypeToolResult {
			lastToolResultIdx = i
			break
		}
	}

	if lastToolResultIdx == -1 {
		return m, false
	}

	m.expanded[lastToolResultIdx] = !m.expanded[lastToolResultIdx]
	return m, true
}

// HasExpandedResults returns true if any tool result is currently expanded.
func (m OutputModel) HasExpandedResults() bool {
	for _, v := range m.expanded {
		if v {
			return true
		}
	}
	return false
}
