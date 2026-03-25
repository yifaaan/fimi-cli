package shell

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
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
	var p *tea.Program
	eventfulRunner := bindRuntimeEvents(runner, func(msg tea.Msg) {
		if p != nil {
			p.Send(msg)
		}
	})

	// Create initial model
	m := NewModel(eventfulRunner, store, modelName, systemPrompt)

	// Create bubbletea program
	// WithoutSignalHandler lets us handle signals ourselves
	p = tea.NewProgram(
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

// bindRuntimeEvents injects a sink that forwards runtime events into bubbletea.
// 这样 shell 保持在 UI 适配层消费事件，而 runtime 不需要知道 bubbletea。
func bindRuntimeEvents(runner runtime.Runner, send func(tea.Msg)) runtime.Runner {
	if send == nil {
		return runner
	}

	sink := runtimeevents.SinkFunc(func(ctx context.Context, event runtimeevents.Event) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		send(runtimeEventMsg{event: event})
		return nil
	})

	return runner.WithEventSink(sink)
}
