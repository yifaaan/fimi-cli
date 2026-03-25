package shell

import (
	"bytes"

	tea "github.com/charmbracelet/bubbletea"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
)

// Model is the bubbletea model for the shell UI.
type Model struct {
	// Dependencies (injected at construction)
	runner       runtime.Runner
	store        contextstore.Context
	modelName    string
	systemPrompt string

	// Prompt state
	prompt string
	width  int

	// Runtime state
	running bool
	result  runtime.Result
	err     error

	// Output buffer (accumulates streaming text)
	output            bytes.Buffer
	assistantLineOpen bool
	assistantTurnOpen bool

	// UI state
	showHelp bool
	status   runStatus
}

// runStatus stores the minimal live execution state shown above the prompt.
type runStatus struct {
	Step             int
	ActiveTool       string
	ActiveToolDetail string
	LastToolResult   string
	LastToolError    bool
	ContextUsage     float64
}

// NewModel creates a new shell model with the given dependencies.
func NewModel(
	runner runtime.Runner,
	store contextstore.Context,
	modelName string,
	systemPrompt string,
) Model {
	return Model{
		runner:       runner,
		store:        store,
		modelName:    modelName,
		systemPrompt: systemPrompt,
	}
}

// reset clears the per-run live status when starting a fresh prompt.
func (s *runStatus) reset() {
	*s = runStatus{}
}

// SetWidth updates the terminal width for rendering.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}
