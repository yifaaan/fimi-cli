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
	m.prompt = "" // Clear input after submission

	return m, m.runPromptCmd(input)
}

// handlePromptInput starts the runtime with the given prompt text.
func (m Model) handlePromptInput(msg promptInputMsg) (tea.Model, tea.Cmd) {
	m.running = true
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
		for _, record := range step.AppendedRecords {
			m.appendToRecord(&m.output, record)
		}
	}

	return m, nil
}

// appendToRecord appends a contextstore.TextRecord to the output buffer.
func (m *Model) appendToRecord(sb *strings.Builder, record contextstore.TextRecord) {
	switch record.Role {
	case "assistant":
		sb.WriteString(record.Content)
		sb.WriteString("\n")
	case "tool":
		sb.WriteString("[tool result] ")
		sb.WriteString(record.Content)
		sb.WriteString("\n")
	}
}

// handleInterrupt handles interrupt signal by canceling running task.
func (m Model) handleInterrupt() (tea.Model, tea.Cmd) {
	if m.running {
		m.running = false
		m.err = fmt.Errorf("interrupted")
	}
	return m, nil
}

// appendToOutput formats and appends a runtime event to the output buffer.
// It handles TextPart, ToolCall, ToolResult, StepBegin, StepInterrupted, and StatusUpdate events.
func (m *Model) appendToOutput(event runtimeevents.Event) {
	switch e := event.(type) {
	case runtimeevents.TextPart:
		m.output.WriteString(e.Text)

	case runtimeevents.ToolCall:
		summary := toolCallSummary(e)
		m.output.WriteString(fmt.Sprintf("\n[tool] %s %s\n", e.Name, summary))

	case runtimeevents.ToolResult:
		prefix := "[result]"
		if e.IsError {
			prefix = "[error]"
		}
		output := strings.TrimSpace(e.Output)
		if output == "" {
			m.output.WriteString(fmt.Sprintf("%s %s\n", prefix, e.ToolName))
		} else if strings.Contains(output, "\n") {
			m.output.WriteString(fmt.Sprintf("%s %s\n%s\n", prefix, e.ToolName, output))
		} else {
			m.output.WriteString(fmt.Sprintf("%s %s %s\n", prefix, e.ToolName, output))
		}

	case runtimeevents.StepBegin:
		// No output for step begin in minimal version

	case runtimeevents.StepInterrupted:
		m.output.WriteString("\n[interrupted]\n")

	case runtimeevents.StatusUpdate:
		// No output for status in minimal version
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
