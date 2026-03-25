package shell

import (
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
