package shell

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
)

// Update handles incoming messages and updates the model.
// It implements the tea.Model interface.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.SetWidth(msg.Width)
		return m, nil

	case promptInputMsg:
		return m.handlePromptInput(msg)

	case runtimeEventMsg:
		return m.handleRuntimeEvent(msg)

	case runtimeDoneMsg:
		return m.handleRuntimeDone(msg)

	case interruptMsg:
		return m.handleInterrupt()

	case showHelpMsg:
		m.showHelp = true
		return m, nil
	}

	return m, nil
}

// handleKey processes keyboard input.
// If runtime is running, only Ctrl+C is allowed to interrupt.
// Otherwise, typing, backspace, enter, and Ctrl+D are handled.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		// Ctrl+C always allowed - if running, interrupt immediately; otherwise clear prompt
		if m.running {
			m.running = false
			m.err = fmt.Errorf("interrupted")
			return m, func() tea.Msg { return interruptMsg{} }
		}
		m.prompt = ""
		m.showHelp = false
		m.err = nil
		return m, nil

	case tea.KeyCtrlD:
		// Exit shell (only when not running)
		if m.running {
			return m, nil
		}
		return m, tea.Quit

	case tea.KeyEnter:
		if m.running {
			return m, nil
		}
		return m.handleSubmit()

	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.running {
			return m, nil
		}
		if len(m.prompt) > 0 {
			m.prompt = m.prompt[:len(m.prompt)-1]
		}
		return m, nil

	case tea.KeyRunes:
		if m.running {
			return m, nil
		}
		m.prompt += string(msg.Runes)
		return m, nil
	}

	return m, nil
}

// handleSubmit handles Enter key press.
// It checks for meta commands first, then starts the runtime.
func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.prompt)
	if input == "" {
		return m, nil
	}

	// Clear help overlay and error
	m.showHelp = false
	m.err = nil

	// Check for meta command
	if cmdName, ok := isMetaCommand(input); ok {
		m.prompt = ""
		cmd, _ := getMetaCommand(cmdName)
		return m, cmd.Execute(&m)
	}

	// Start runtime
	m.running = true
	m.status.reset()
	m.appendUserTurn(input)
	m.prompt = "" // Clear input after submission

	return m, m.runPromptCmd(input)
}

// handlePromptInput starts the runtime with the given prompt text.
func (m Model) handlePromptInput(msg promptInputMsg) (tea.Model, tea.Cmd) {
	m.running = true
	m.status.reset()
	m.appendUserTurn(strings.TrimSpace(msg.text))
	return m, m.runPromptCmd(msg.text)
}

// runPromptCmd returns a tea.Cmd that runs the runtime and sends runtimeDoneMsg.
// The cmd executes the runner in a goroutine since runtime.Run is blocking.
func (m Model) runPromptCmd(prompt string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.runner.Run(
			context.Background(),
			m.store,
			runtime.Input{
				Prompt:       prompt,
				Model:        m.modelName,
				SystemPrompt: m.systemPrompt,
			},
		)
		return runtimeDoneMsg{result: result, err: err}
	}
}

// handleRuntimeEvent appends event output to the output buffer.
func (m Model) handleRuntimeEvent(msg runtimeEventMsg) (tea.Model, tea.Cmd) {
	m.applyRuntimeStatus(msg.event)
	m.appendToOutput(msg.event)
	return m, nil
}

// handleRuntimeDone marks the runtime as complete and handles any error.
// It also processes the result records to populate the output buffer
// when event streaming wasn't connected.
func (m Model) handleRuntimeDone(msg runtimeDoneMsg) (tea.Model, tea.Cmd) {
	m.running = false
	if msg.err != nil {
		m.err = msg.err
	}

	// Process result records into output buffer
	// This handles the case when event streaming wasn't connected
	for _, step := range msg.result.Steps {
		if step.TextStreamed {
			continue
		}
		for _, record := range step.AppendedRecords {
			m.appendToRecord(record)
		}
	}
	m.closeAssistantLine()

	return m, nil
}

// appendToRecord appends a contextstore.TextRecord to the output buffer.
func (m *Model) appendToRecord(record contextstore.TextRecord) {
	switch record.Role {
	case "assistant":
		m.appendAssistantText(record.Content)
		m.closeAssistantLine()
	case "tool":
		m.appendAssistantMetaLine("tool result:", record.Content)
	}
}

// handleInterrupt handles interrupt signal by canceling running task.
func (m Model) handleInterrupt() (tea.Model, tea.Cmd) {
	if m.running {
		m.running = false
		m.err = fmt.Errorf("interrupted")
	}
	m.closeAssistantLine()
	return m, nil
}

// appendToOutput formats and appends a runtime event to the output buffer.
// It handles TextPart, ToolCall, ToolResult, StepBegin, StepInterrupted, and StatusUpdate events.
func (m *Model) appendToOutput(event runtimeevents.Event) {
	switch e := event.(type) {
	case runtimeevents.TextPart:
		m.appendAssistantText(e.Text)

	case runtimeevents.ToolCall:
		summary := toolCallSummary(e)
		m.appendAssistantMetaLine("tool:", strings.TrimSpace(strings.Join([]string{e.Name, summary}, " ")))

	case runtimeevents.ToolCallPart:
		delta := strings.TrimSpace(e.Delta)
		if delta == "" {
			return
		}
		m.appendAssistantMetaLine("tool args:", strings.TrimSpace(strings.Join([]string{e.ToolCallID, clampLine(delta, 60)}, " ")))

	case runtimeevents.ToolResult:
		prefix := "[result]"
		if e.IsError {
			prefix = "[error]"
		}
		output := strings.TrimSpace(e.Output)
		if output == "" {
			m.appendAssistantMetaLine(strings.Trim(prefix, "[]")+":", e.ToolName)
		} else if strings.Contains(output, "\n") {
			m.appendAssistantMetaLine(strings.Trim(prefix, "[]")+":", e.ToolName)
			for _, line := range strings.Split(output, "\n") {
				line = strings.TrimRight(line, "\r")
				if line == "" {
					m.output.WriteString("    \n")
					continue
				}
				m.output.WriteString("    ")
				m.output.WriteString(line)
				m.output.WriteString("\n")
			}
		} else {
			m.appendAssistantMetaLine(strings.Trim(prefix, "[]")+":", strings.TrimSpace(strings.Join([]string{e.ToolName, output}, " ")))
		}

	case runtimeevents.StepBegin:
		// No output for step begin in minimal version

	case runtimeevents.StepInterrupted:
		m.appendAssistantMetaLine("status:", "interrupted")

	case runtimeevents.StatusUpdate:
		// No output for status in minimal version
	}
}

func (m *Model) appendUserTurn(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	m.closeAssistantLine()
	m.assistantTurnOpen = false
	m.ensureTurnSeparator()
	m.output.WriteString("user: ")
	m.output.WriteString(text)
	m.output.WriteString("\n")
}

func (m *Model) appendAssistantText(text string) {
	if text == "" {
		return
	}

	if !m.assistantLineOpen {
		if !m.assistantTurnOpen {
			m.output.WriteString("assistant: ")
			m.assistantTurnOpen = true
		} else {
			m.output.WriteString("assistant: ")
		}
		m.assistantLineOpen = true
	}
	m.output.WriteString(text)
}

func (m *Model) closeAssistantLine() {
	if !m.assistantLineOpen {
		return
	}

	m.output.WriteString("\n")
	m.assistantLineOpen = false
}

func (m *Model) ensureAssistantTurnBlock() {
	if m.assistantTurnOpen {
		return
	}

	if m.output.Len() > 0 && !strings.HasSuffix(m.output.String(), "\n") {
		m.output.WriteString("\n")
	}
	m.output.WriteString("assistant:\n")
	m.assistantTurnOpen = true
}

func (m *Model) appendAssistantMetaLine(label, text string) {
	m.closeAssistantLine()
	m.ensureAssistantTurnBlock()
	m.output.WriteString("  ")
	m.output.WriteString(strings.TrimSpace(label))
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		m.output.WriteString(" ")
		m.output.WriteString(trimmed)
	}
	m.output.WriteString("\n")
}

func (m *Model) ensureTurnSeparator() {
	if m.output.Len() == 0 {
		return
	}

	output := m.output.String()
	if !strings.HasSuffix(output, "\n") {
		m.output.WriteString("\n")
		output = m.output.String()
	}
	if !strings.HasSuffix(output, "\n\n") {
		m.output.WriteString("\n")
	}
}

// applyRuntimeStatus folds raw runtime events into a compact UI state.
func (m *Model) applyRuntimeStatus(event runtimeevents.Event) {
	switch e := event.(type) {
	case runtimeevents.StepBegin:
		m.status.Step = e.Number
		m.status.ActiveTool = ""
		m.status.ActiveToolDetail = ""
		m.status.LastToolResult = ""
		m.status.LastToolError = false
	case runtimeevents.ToolCall:
		m.status.ActiveTool = e.Name
		m.status.ActiveToolDetail = toolCallSummary(e)
	case runtimeevents.ToolResult:
		m.status.ActiveTool = ""
		m.status.ActiveToolDetail = ""
		m.status.LastToolError = e.IsError
		if strings.TrimSpace(e.Output) == "" {
			m.status.LastToolResult = e.ToolName
			return
		}
		m.status.LastToolResult = fmt.Sprintf("%s %s", e.ToolName, clampLine(e.Output, 60))
	case runtimeevents.StepInterrupted:
		m.status.ActiveTool = ""
		m.status.ActiveToolDetail = ""
		m.status.LastToolError = true
		m.status.LastToolResult = "interrupted"
	case runtimeevents.StatusUpdate:
		m.status.ContextUsage = e.Status.ContextUsage
	}
}

// toolCallSummary extracts a short summary from a tool call for display.
func toolCallSummary(e runtimeevents.ToolCall) string {
	if e.Subtitle != "" {
		return clampLine(e.Subtitle, 60)
	}
	return clampLine(e.Arguments, 60)
}

// clampLine truncates a string to maxLen characters.
func clampLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
