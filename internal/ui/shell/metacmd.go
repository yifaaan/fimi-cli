package shell

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MetaCommand represents a built-in shell command.
type MetaCommand struct {
	Name        string
	Description string
	Execute     func(m *Model) tea.Cmd
}

// metaCommands holds all available meta commands.
var metaCommands = map[string]MetaCommand{
	"exit": {
		Name:        "exit",
		Description: "Exit the shell",
		Execute:     cmdExit,
	},
	"help": {
		Name:        "help",
		Description: "Show available commands",
		Execute:     cmdHelp,
	},
}

// getMetaCommand returns the command by name (case insensitive).
func getMetaCommand(name string) (MetaCommand, bool) {
	cmd, ok := metaCommands[strings.ToLower(name)]
	return cmd, ok
}

// isMetaCommand checks if input starts with "/" and matches a known command.
func isMetaCommand(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	name := strings.ToLower(strings.TrimPrefix(input, "/"))
	_, exists := metaCommands[name]
	if !exists {
		return "", false
	}
	return name, true
}

// cmdExit returns tea.Quit to terminate the program.
func cmdExit(m *Model) tea.Cmd {
	return tea.Quit
}

// cmdHelp sets showHelp to true via a command.
func cmdHelp(m *Model) tea.Cmd {
	return func() tea.Msg {
		return showHelpMsg{}
	}
}

// showHelpMsg is an internal message to show help.
type showHelpMsg struct{}

// renderHelp returns formatted help text.
func renderHelp() string {
	var lines []string
	lines = append(lines, "\x1b[1mCommands:\x1b[0m")
	lines = append(lines, "  /exit    Exit the shell")
	lines = append(lines, "  /help    Show this help message")
	lines = append(lines, "")
	lines = append(lines, "\x1b[1mShortcuts:\x1b[0m")
	lines = append(lines, "  Ctrl+C   Clear prompt (or cancel running task)")
	lines = append(lines, "  Ctrl+D   Exit shell")
	return strings.Join(lines, "\n")
}
