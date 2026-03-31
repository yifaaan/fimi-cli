package shell

import (
	"context"
	"fmt"
	"strings"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/styles"
	"fimi-cli/internal/wire"

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
	// 当前重试等待状态
	Retry *runtimeevents.RetryStatus
	// 助手流式输出文本
	AssistantText string
	// 当前工具调用信息
	CurrentTool *ToolCallInfo
	// 当前 step 内所有工具状态（按到达顺序保留）
	toolsByID   map[string]*ToolCallInfo
	toolOrder   []string
	toolLineIdx map[string]int
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
		m.toolsByID = make(map[string]*ToolCallInfo)
		m.toolOrder = nil
		m.toolLineIdx = make(map[string]int)
		m.Retry = nil
		m.stepLines = []TranscriptLine{{
			Type:    LineTypeSystem,
			Content: fmt.Sprintf("Step %d", e.Number),
		}}
		m.Interrupted = false

	case runtimeevents.StatusUpdate:
		m.ContextUsage = e.Status.ContextUsage
		m.Retry = cloneRetryStatus(e.Status.Retry)

	case runtimeevents.TextPart:
		m.AssistantText += e.Text
		m.appendAssistantText(e.Text)

	case runtimeevents.ToolCall:
		tool := &ToolCallInfo{
			ID:     e.ID,
			Name:   e.Name,
			Status: ToolStatusRunning,
			Args:   toolCallDisplaySummary(e.Name, e.Subtitle, e.Arguments),
		}
		if m.toolsByID == nil {
			m.toolsByID = make(map[string]*ToolCallInfo)
		}
		if m.toolLineIdx == nil {
			m.toolLineIdx = make(map[string]int)
		}
		m.toolsByID[e.ID] = tool
		m.toolOrder = append(m.toolOrder, e.ID)
		m.CurrentTool = tool
		m.appendToolCallLine(tool)

	case runtimeevents.ToolCallPart:
		if tool := m.toolsByID[e.ToolCallID]; tool != nil {
			tool.Args += e.Delta
			m.updateToolCallLine(e.ToolCallID)
			if m.CurrentTool != nil && m.CurrentTool.ID == e.ToolCallID {
				m.CurrentTool = tool
			}
		}

	case runtimeevents.ToolResult:
		if tool := m.toolsByID[e.ToolCallID]; tool != nil {
			tool.Output = e.Output
			tool.IsError = e.IsError
			if e.IsError {
				tool.Status = ToolStatusError
			} else {
				tool.Status = ToolStatusCompleted
			}
			m.appendToolResultLine(tool)
			m.CurrentTool = m.latestRunningTool()
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
	m.Retry = nil
	m.AssistantText = ""
	m.CurrentTool = nil
	m.toolsByID = nil
	m.toolOrder = nil
	m.toolLineIdx = nil
	m.stepLines = nil
	m.Interrupted = false
	return m
}

func cloneRetryStatus(retry *runtimeevents.RetryStatus) *runtimeevents.RetryStatus {
	if retry == nil {
		return nil
	}

	copy := *retry
	return &copy
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

func (m *RuntimeModel) appendToolCallLine(tool *ToolCallInfo) {
	if tool == nil {
		return
	}

	m.stepLines = append(m.stepLines, TranscriptLine{
		Type:    LineTypeToolCall,
		Content: formatToolCallLine(*tool),
	})
	if m.toolLineIdx == nil {
		m.toolLineIdx = make(map[string]int)
	}
	m.toolLineIdx[tool.ID] = len(m.stepLines) - 1
}

func (m *RuntimeModel) updateToolCallLine(toolCallID string) {
	tool := m.toolsByID[toolCallID]
	if tool == nil {
		return
	}
	if idx, ok := m.toolLineIdx[toolCallID]; ok && idx >= 0 && idx < len(m.stepLines) {
		m.stepLines[idx].Content = formatToolCallLine(*tool)
	}
}

func (m *RuntimeModel) appendToolResultLine(tool *ToolCallInfo) {
	if tool == nil || tool.Output == "" {
		return
	}

	lineType := LineTypeToolResult
	if tool.IsError {
		lineType = LineTypeError
	}
	m.stepLines = append(m.stepLines, TranscriptLine{
		Type:    lineType,
		Content: strings.TrimSpace(tool.Output),
	})
}

func (m RuntimeModel) latestRunningTool() *ToolCallInfo {
	for i := len(m.toolOrder) - 1; i >= 0; i-- {
		tool := m.toolsByID[m.toolOrder[i]]
		if tool != nil && tool.Status == ToolStatusRunning {
			return tool
		}
	}
	return nil
}

// wireErrorMsg wraps wire receive errors for tea.Msg.
type wireErrorMsg struct {
	Err error
}

// approvalRequestMsg wraps approval requests for tea.Msg.
type approvalRequestMsg struct {
	Request *wire.ApprovalRequest
}

// approvalResolveMsg wraps approval resolution for tea.Msg.
type approvalResolveMsg struct {
	ID       string
	Response wire.ApprovalResponse
}

// wireReceiveLoop returns a tea.Cmd that receives from wire and converts to tea.Msg.
func (m Model) wireReceiveLoop() tea.Cmd {
	return func() tea.Msg {
		msg, err := m.wire.Receive(context.Background())
		if err != nil {
			// Return nil on error to stop the loop
			return nil
		}

		switch msg := msg.(type) {
		case wire.EventMessage:
			return eventToTeaMsg(msg.Event)
		case *wire.ApprovalRequest:
			return approvalRequestMsg{Request: msg}
		case wire.ToastMessage:
			return ToastAddMsg{Toast: Toast{
				Level:   parseToastLevel(msg.Level),
				Message: msg.Message,
				Detail:  msg.Detail,
				Action:  msg.Action,
			}}
		default:
			return nil
		}
	}
}

// eventToTeaMsg converts runtime events to existing tea.Msg types.
func eventToTeaMsg(event runtimeevents.Event) tea.Msg {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		return RuntimeEventMsg{Event: e}
	case runtimeevents.StepInterrupted:
		return RuntimeEventMsg{Event: e}
	case runtimeevents.StatusUpdate:
		return RuntimeEventMsg{Event: e}
	case runtimeevents.TextPart:
		return RuntimeEventMsg{Event: e}
	case runtimeevents.ToolCall:
		return RuntimeEventMsg{Event: e}
	case runtimeevents.ToolCallPart:
		return RuntimeEventMsg{Event: e}
	case runtimeevents.ToolResult:
		return RuntimeEventMsg{Event: e}
	default:
		return nil
	}
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
