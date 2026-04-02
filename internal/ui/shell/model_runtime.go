package shell

import (
	"context"
	"strings"
	"time"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui/shell/styles"
	"fimi-cli/internal/wire"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type RuntimeModel struct {
	Step          int
	ContextUsage  float64
	Retry         *runtimeevents.RetryStatus
	AssistantText string
	CurrentTool   *ToolCallInfo
	toolsByID     map[string]*ToolCallInfo
	toolOrder     []string
	builder       transcriptBuilder
	Interrupted   bool
	spinner       spinner.Model
}

type ToolCallInfo struct {
	ID      string
	Name    string
	Status  ToolStatus
	Args    string
	Output  string
	IsError bool
}

type ToolStatus int

const (
	ToolStatusPending ToolStatus = iota
	ToolStatusRunning
	ToolStatusCompleted
	ToolStatusError
)

func NewRuntimeModel() RuntimeModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.ColorAccent)

	return RuntimeModel{
		spinner: s,
		builder: newTranscriptBuilder(),
	}
}

func (m RuntimeModel) ApplyEvent(event runtimeevents.Event) RuntimeModel {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		m.Step = e.Number
		m.AssistantText = ""
		m.CurrentTool = nil
		m.toolsByID = make(map[string]*ToolCallInfo)
		m.toolOrder = nil
		m.Retry = nil
		m.builder.resetTurn(time.Now())
		m.Interrupted = false

	case runtimeevents.StatusUpdate:
		m.ContextUsage = e.Status.ContextUsage
		m.Retry = cloneRetryStatus(e.Status.Retry)

	case runtimeevents.TextPart:
		m.AssistantText += e.Text

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
		m.toolsByID[e.ID] = tool
		m.toolOrder = append(m.toolOrder, e.ID)
		m.CurrentTool = tool

	case runtimeevents.ToolCallPart:
		if tool := m.toolsByID[e.ToolCallID]; tool != nil {
			tool.Args += e.Delta
			if m.CurrentTool != nil && m.CurrentTool.ID == e.ToolCallID {
				m.CurrentTool = tool
			}
		}

	case runtimeevents.ToolResult:
		if tool := m.toolsByID[e.ToolCallID]; tool != nil {
			tool.Output = toolResultDisplayOutput(e.Output, e.DisplayOutput)
			tool.IsError = e.IsError
			if e.IsError {
				tool.Status = ToolStatusError
			} else {
				tool.Status = ToolStatusCompleted
			}
			m.CurrentTool = m.latestRunningTool()
		}

	case runtimeevents.StepInterrupted:
		m.Interrupted = true
	}

	m.builder.applyEvent(event)
	return m
}

func (m RuntimeModel) ApplyApprovalRequest(req *wire.ApprovalRequest, selection int) RuntimeModel {
	m.builder.upsertApproval(req, selection)
	return m
}

func (m RuntimeModel) UpdateApprovalSelection(id string, selection int) RuntimeModel {
	m.builder.updateApprovalSelection(id, selection)
	return m
}

func (m RuntimeModel) ResolveApproval(id string, response wire.ApprovalResponse) RuntimeModel {
	m.builder.resolveApproval(id, response)
	return m
}

func (m RuntimeModel) Advance(now time.Time) RuntimeModel {
	m.builder.tick(now)
	return m
}

func (m RuntimeModel) Reset() RuntimeModel {
	m.Step = 0
	m.ContextUsage = 0
	m.Retry = nil
	m.AssistantText = ""
	m.CurrentTool = nil
	m.toolsByID = nil
	m.toolOrder = nil
	m.builder = newTranscriptBuilder()
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

func (m RuntimeModel) ToBlocks() []TranscriptBlock {
	return m.builder.snapshot()
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

type wireErrorMsg struct {
	Err error
}

type approvalRequestMsg struct {
	Request *wire.ApprovalRequest
}

type approvalResolveMsg struct {
	ID       string
	Response wire.ApprovalResponse
}

func (m Model) wireReceiveLoop() tea.Cmd {
	return func() tea.Msg {
		msg, err := m.wire.Receive(context.Background())
		if err != nil {
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

func toolResultDisplayOutput(output string, displayOutput string) string {
	if strings.TrimSpace(displayOutput) != "" {
		return displayOutput
	}
	return output
}

func toolResultPreviewOutput(toolName string, output string, displayOutput string) string {
	switch toolName {
	case "read_file", "glob", "grep", "search_web", "fetch_url":
		if strings.TrimSpace(output) != "" {
			return output
		}
	}
	return toolResultDisplayOutput(output, displayOutput)
}

func (m RuntimeModel) SpinnerCmd() tea.Cmd {
	return m.spinner.Tick
}

func (m RuntimeModel) UpdateSpinner(msg tea.Msg) (RuntimeModel, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m RuntimeModel) SpinnerView() string {
	return m.spinner.View()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
