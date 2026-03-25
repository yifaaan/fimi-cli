package shell

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Lipgloss styles for status indicators.
var (
	greenDotStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")). // Green
			Bold(true)

	yellowDotStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // Yellow
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // Red
			Bold(true)
)

// greenDot is the rendered green status indicator for testing.
var greenDot = greenDotStyle.Render("●")

// yellowDot is the rendered yellow status indicator for testing.
var yellowDot = yellowDotStyle.Render("●")

// View returns the string representation of the shell UI.
func (m Model) View() string {
	var sb strings.Builder

	// Include previous output buffer content
	if m.output.Len() > 0 {
		sb.WriteString(m.output.String())
		if !strings.HasSuffix(m.output.String(), "\n") {
			sb.WriteString("\n")
		}
	}

	if statusText := m.renderStatusBlock(); statusText != "" {
		sb.WriteString(statusText)
	}

	// Append error text if present
	if m.err != nil {
		sb.WriteString(errorStyle.Render(m.err.Error()))
		sb.WriteString("\n")
	}

	// Append help text if shown
	if m.showHelp {
		sb.WriteString(renderHelp())
		sb.WriteString("\n")
	}

	// Status dot and prompt line
	statusDot := greenDotStyle.Render("●")
	if m.running {
		statusDot = yellowDotStyle.Render("●")
	}
	sb.WriteString(statusDot + " > " + m.prompt)

	return sb.String()
}

func (m Model) renderStatusBlock() string {
	lines := make([]string, 0, 3)

	if m.status.Step > 0 {
		lines = append(lines, "step: "+clampLine(formatStepLabel(m.status.Step, m.running), 60))
	}
	if m.status.ActiveTool != "" {
		line := "tool: " + m.status.ActiveTool
		if m.status.ActiveToolDetail != "" {
			line += " " + m.status.ActiveToolDetail
		}
		lines = append(lines, clampLine(line, 80))
	}
	if m.status.LastToolResult != "" {
		prefix := "result: "
		if m.status.LastToolError {
			prefix = "error: "
		}
		lines = append(lines, prefix+clampLine(m.status.LastToolResult, 72))
	}
	if usageText := formatContextUsage(m.status.ContextUsage); usageText != "" {
		lines = append(lines, "context: "+usageText)
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n") + "\n"
}

func formatStepLabel(step int, running bool) string {
	if running {
		return "running #" + strconv.Itoa(step)
	}
	return "finished #" + strconv.Itoa(step)
}

func formatContextUsage(usage float64) string {
	if usage <= 0 {
		return ""
	}

	bounded := math.Max(0, math.Min(usage, 1))
	return strconv.Itoa(int(math.Round(bounded*100))) + "%"
}
