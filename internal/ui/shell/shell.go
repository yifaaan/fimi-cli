package shell

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
)

// Run starts the interactive shell.
// It handles signals gracefully and manages the bubbletea event loop.
func Run(
	ctx context.Context,
	runner runtime.Runner,
	store contextstore.Context,
	modelName string,
	systemPrompt string,
) error {
	// Create initial model
	m := NewModel(runner, store, modelName, systemPrompt)

	// Create bubbletea program
	// WithoutSignalHandler lets us handle signals ourselves
	p := tea.NewProgram(
		m,
		tea.WithoutSignalHandler(),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// Goroutine to forward signals as messages
	go func() {
		for range sigChan {
			p.Send(interruptMsg{})
		}
	}()

	// Handle context cancellation
	go func() {
		<-ctx.Done()
		p.Send(interruptMsg{})
	}()

	// Run bubbletea
	_, err := p.Run()
	return err
}
