package shell

import (
	"fmt"
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/styles"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RuntimeModel 跟踪运行时的实时状态，用于 UI 显示。
type RuntimeModel struct {
	// 当前步骤号
	Step int
	// 上下文使用率 (0.0 - 1.0)
	ContextUsage float64
	// 助手流式输出文本
	AssistantText string
	// 当前工具调用信息
	CurrentTool *ToolCallInfo
	// 当前 step 已累积的 transcript 行
	stepLines []TranscriptLine
	// Spinner 动画
	spinner spinner.Model
	// 是否被中断
	Interrupted bool
}

// ToolCallInfo 包含工具调用的详细信息。
type ToolCallInfo struct {
	ID      string
	Name    string
	Status  ToolStatus
	Args    string
	Output  string
	IsError bool
}

// ToolStatus 表示工具调用的状态。
type ToolStatus int

const (
	// ToolStatusPending 等待执行
	ToolStatusPending ToolStatus = iota
	// ToolStatusRunning 正在执行
	ToolStatusRunning
	// ToolStatusCompleted 执行完成
	ToolStatusCompleted
	// ToolStatusError 执行失败
	ToolStatusError
)

// NewRuntimeModel 创建一个新的运行时模型。
func NewRuntimeModel() RuntimeModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.ColorAccent)

	return RuntimeModel{
		spinner: s,
	}
}

// ApplyEvent 应用一个 runtime 事件到模型。
func (m RuntimeModel) ApplyEvent(event runtimeevents.Event) RuntimeModel {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		m.Step = e.Number
		m.AssistantText = ""
		m.CurrentTool = nil
		m.stepLines = []TranscriptLine{{
			Type:    LineTypeSystem,
			Content: fmt.Sprintf("Step %d", e.Number),
		}}
		m.Interrupted = false

	case runtimeevents.StatusUpdate:
		m.ContextUsage = e.Status.ContextUsage

	case runtimeevents.TextPart:
		m.AssistantText += e.Text
		m.appendAssistantText(e.Text)

	case runtimeevents.ToolCall:
		m.CurrentTool = &ToolCallInfo{
			ID:     e.ID,
			Name:   e.Name,
			Status: ToolStatusRunning,
			Args:   toolCallDisplaySummary(e.Name, e.Subtitle, e.Arguments),
		}
		m.appendToolCallLine()

	case runtimeevents.ToolCallPart:
		if m.CurrentTool != nil {
			m.CurrentTool.Args += e.Delta
			m.updateCurrentToolCallLine()
		}

	case runtimeevents.ToolResult:
		if m.CurrentTool != nil {
			m.CurrentTool.Output = e.Output
			m.CurrentTool.IsError = e.IsError
			if e.IsError {
				m.CurrentTool.Status = ToolStatusError
			} else {
				m.CurrentTool.Status = ToolStatusCompleted
			}
			m.appendToolResultLine()
		}

	case runtimeevents.StepInterrupted:
		m.Interrupted = true
		m.stepLines = append(m.stepLines, TranscriptLine{
			Type:    LineTypeSystem,
			Content: "Interrupted",
		})
	}

	return m
}

// Reset 重置运行时状态。
func (m RuntimeModel) Reset() RuntimeModel {
	m.Step = 0
	m.ContextUsage = 0
	m.AssistantText = ""
	m.CurrentTool = nil
	m.stepLines = nil
	m.Interrupted = false
	return m
}

// ToLines 将当前状态转换为 transcript 行。
func (m RuntimeModel) ToLines() []TranscriptLine {
	return append([]TranscriptLine(nil), m.stepLines...)
}

func (m *RuntimeModel) appendAssistantText(delta string) {
	if delta == "" {
		return
	}
	if len(m.stepLines) > 0 && m.stepLines[len(m.stepLines)-1].Type == LineTypeAssistant {
		m.stepLines[len(m.stepLines)-1].Content += delta
		return
	}

	m.stepLines = append(m.stepLines, TranscriptLine{
		Type:    LineTypeAssistant,
		Content: delta,
	})
}

func (m *RuntimeModel) appendToolCallLine() {
	if m.CurrentTool == nil {
		return
	}

	m.stepLines = append(m.stepLines, TranscriptLine{
		Type:    LineTypeToolCall,
		Content: formatToolCallLine(*m.CurrentTool),
	})
}

func (m *RuntimeModel) updateCurrentToolCallLine() {
	if m.CurrentTool == nil {
		return
	}
	for i := len(m.stepLines) - 1; i >= 0; i-- {
		if m.stepLines[i].Type == LineTypeToolCall {
			m.stepLines[i].Content = formatToolCallLine(*m.CurrentTool)
			return
		}
	}
}

func (m *RuntimeModel) appendToolResultLine() {
	if m.CurrentTool == nil || m.CurrentTool.Output == "" {
		return
	}

	lineType := LineTypeToolResult
	if m.CurrentTool.IsError {
		lineType = LineTypeError
	}
	m.stepLines = append(m.stepLines, TranscriptLine{
		Type:    lineType,
		Content: toolResultSummary(*m.CurrentTool),
	})
}

func formatToolCallLine(tool ToolCallInfo) string {
	summary := strings.TrimSpace(tool.Args)
	if summary == "" {
		return tool.Name
	}

	switch tool.Name {
	case "bash":
		return "Bash(" + summary + ")"
	case "read_file":
		return summary
	case "write_file":
		return strings.Replace(summary, "Wrote ", "Write(", 1) + ")"
	case "replace_file", "patch_file":
		summary = strings.Replace(summary, "Updated ", "", 1)
		summary = strings.Replace(summary, "Patched ", "", 1)
		return "Update(" + summary + ")"
	default:
		return summary
	}
}

func toolResultSummary(tool ToolCallInfo) string {
	output := strings.TrimSpace(tool.Output)
	if output == "" {
		if tool.IsError {
			return "Error"
		}
		return "No output"
	}

	lines := strings.Split(output, "\n")
	preview := strings.TrimSpace(lines[0])
	if preview == "" {
		if tool.IsError {
			return "Error"
		}
		return "No output"
	}
	if len(preview) > 80 {
		preview = preview[:77] + "..."
	}
	return preview
}

func toolCallDisplaySummary(name string, subtitle string, arguments string) string {
	if summary := strings.TrimSpace(subtitle); summary != "" {
		return summary
	}

	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return name
	}

	return name + " " + arguments
}

// SpinnerCmd 返回 spinner 动画命令。
func (m RuntimeModel) SpinnerCmd() tea.Cmd {
	return m.spinner.Tick
}

// UpdateSpinner 推进 spinner 动画到下一帧。
func (m RuntimeModel) UpdateSpinner(msg tea.Msg) (RuntimeModel, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// SpinnerView 渲染 spinner 视图。
func (m RuntimeModel) SpinnerView() string {
	return m.spinner.View()
}

// ToolCardView 渲染工具卡片视图。
func (m RuntimeModel) ToolCardView(width int) string {
	if m.CurrentTool == nil {
		return ""
	}

	tool := m.CurrentTool
	var b strings.Builder

	// 状态图标
	var icon string
	var color lipgloss.Color
	switch tool.Status {
	case ToolStatusRunning:
		icon = " "
		color = styles.ColorWarning
	case ToolStatusCompleted:
		icon = " "
		color = styles.ColorSuccess
	case ToolStatusError:
		icon = " "
		color = styles.ColorError
	default:
		icon = " "
		color = styles.ColorMuted
	}

	// 工具名称
	nameStyle := lipgloss.NewStyle().
		Foreground(color).
		Bold(true)

	header := fmt.Sprintf("%s %s", icon, nameStyle.Render(tool.Name))
	b.WriteString(header)
	b.WriteString("\n")

	// 参数
	if tool.Args != "" {
		argsBox := styles.ToolArgsStyle.
			Width(width - 4).
			Render(truncate(tool.Args, width-6))
		b.WriteString(argsBox)
		b.WriteString("\n")
	}

	// 输出（如果完成）
	if tool.Status == ToolStatusCompleted || tool.Status == ToolStatusError {
		if tool.Output != "" {
			outputBox := styles.ToolOutputStyle.
				Width(width - 4).
				Render(truncate(tool.Output, width-6))
			b.WriteString(outputBox)
		}
	}

	// 卡片容器
	return styles.ToolCardStyle.
		Width(width).
		Render(b.String())
}

// truncate 截断字符串到指定长度。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
