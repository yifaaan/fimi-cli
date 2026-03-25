package shell

import (
	"strings"
	"time"

	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

// NewOutputModel 创建一个新的输出模型。
func NewOutputModel() OutputModel {
	return OutputModel{
		lines:    make([]TranscriptLine, 0),
		pending:  make([]TranscriptLine, 0),
		atBottom: true,
	}
}

// AppendLine 添加一行到 transcript。
func (m OutputModel) AppendLine(line TranscriptLine) OutputModel {
	line.Time = time.Now()
	m.lines = append(m.lines, line)
	return m
}

// SetPending 用最新快照替换实时内容。
func (m OutputModel) SetPending(lines []TranscriptLine) OutputModel {
	m.pending = append([]TranscriptLine(nil), lines...)
	return m
}

// FlushPending 将实时内容刷新到已完成的行。
func (m OutputModel) FlushPending() OutputModel {
	m.lines = append(m.lines, m.pending...)
	m.pending = nil
	return m
}

// Clear 清空 transcript。
func (m OutputModel) Clear() OutputModel {
	m.lines = nil
	m.pending = nil
	return m
}

// Update 处理消息并更新状态。
func (m OutputModel) Update(msg tea.Msg, width, height int) (OutputModel, tea.Cmd) {
	m.width = width
	m.height = height
	return m, nil
}

// View 渲染 transcript 视图。
func (m OutputModel) View() string {
	var allLines []TranscriptLine
	allLines = append(allLines, m.lines...)
	allLines = append(allLines, m.pending...)

	if len(allLines) == 0 {
		return ""
	}

	// 计算可用高度（留出输入区和状态栏）
	availableHeight := m.height - 6
	if availableHeight < 5 {
		availableHeight = 5
	}

	// 只显示最后 N 行
	startIdx := 0
	if len(allLines) > availableHeight {
		startIdx = len(allLines) - availableHeight
	}

	var b strings.Builder
	for i := startIdx; i < len(allLines); i++ {
		line := allLines[i]
		b.WriteString(m.renderLine(line))
		b.WriteString("\n")
	}

	return b.String()
}

// renderLine 渲染单行。
func (m OutputModel) renderLine(line TranscriptLine) string {
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
		content = styles.SystemStyle.Render(line.Content)
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
