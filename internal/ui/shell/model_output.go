package shell

import (
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	// 正在更新的实时内容（流式输出）
	pending []TranscriptLine
	// 视口尺寸
	width  int
	height int
	// 是否自动滚动到底部
	atBottom bool
	// 相对底部的滚动偏移量（按行）
	scrollOffset int
	// 已展开的 ToolResult 行索引（key 为 lines 中的索引）
	expanded map[int]bool
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
		b.WriteString(rows[i])
		b.WriteString("\n")
	}

	return b.String()
}

// renderLine 渲染单行。
// idx 参数用于查找该行的折叠状态（仅对 ToolResult 有效）。
func (m OutputModel) renderLine(line TranscriptLine, idx int) string {
	var prefix string
	var content string

	switch line.Type {
	case LineTypeUser:
		prefix = styles.UserStyle.Render("You:")
		content = line.Content
	case LineTypeAssistant:
		prefix = styles.AssistantStyle.Render("Assistant:")
		content = line.Content
	case LineTypeToolCall:
		prefix = styles.ToolNameStyle.Render("[Tool]")
		content = line.Content
	case LineTypeToolResult:
		prefix = styles.ToolNameStyle.Render("[Result]")
		content = m.renderToolResult(line.Content, idx)
	case LineTypeSystem:
		prefix = styles.SystemStyle.Render("[System]")
		content = styles.SystemStyle.Render(line.Content)
	case LineTypeError:
		prefix = styles.ErrorStyle.Render("[Error]")
		content = styles.ErrorStyle.Render(line.Content)
	default:
		content = line.Content
	}

	if prefix != "" {
		return lipgloss.JoinHorizontal(lipgloss.Top, prefix, " ", content)
	}
	return content
}

// renderToolResult 渲染工具结果，支持折叠显示。
func (m OutputModel) renderToolResult(content string, idx int) string {
	lines := strings.Split(content, "\n")

	// 如果行数不超过阈值，直接显示
	if len(lines) <= foldThreshold {
		return styles.SystemStyle.Render(content)
	}

	// 检查是否已展开
	if m.expanded[idx] {
		return styles.SystemStyle.Render(content)
	}

	// 折叠显示：前 foldThreshold 行 + 提示
	foldedLines := lines[:foldThreshold]
	foldedLines = append(foldedLines,
		styles.HelpStyle.Render(fmt.Sprintf("... (%d more lines, Ctrl+O to expand)", len(lines)-foldThreshold)),
	)
	return strings.Join(foldedLines, "\n")
}

func (m OutputModel) visibleHeight() int {
	availableHeight := m.height - 6
	if availableHeight < 5 {
		availableHeight = 5
	}

	return availableHeight
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
	var allLines []TranscriptLine
	allLines = append(allLines, m.lines...)
	allLines = append(allLines, m.pending...)

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

func (m OutputModel) renderWidth() int {
	if m.width <= 1 {
		return 1
	}

	return m.width
}

// ToggleExpand 切换最后一个 ToolResult 行的折叠状态。
// 返回切换后的模型和是否找到了可切换的行。
func (m OutputModel) ToggleExpand() (OutputModel, bool) {
	// 查找最后一个 ToolResult 类型的行
	lastToolResultIdx := -1
	for i := len(m.lines) - 1; i >= 0; i-- {
		if m.lines[i].Type == LineTypeToolResult {
			lastToolResultIdx = i
			break
		}
	}

	if lastToolResultIdx == -1 {
		return m, false
	}

	// 检查该行是否超过折叠阈值
	lines := strings.Split(m.lines[lastToolResultIdx].Content, "\n")
	if len(lines) <= foldThreshold {
		return m, false
	}

	// 切换状态
	m.expanded[lastToolResultIdx] = !m.expanded[lastToolResultIdx]
	return m, true
}
