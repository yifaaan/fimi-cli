package ui

import (
	"context"
	"errors"
	"fmt"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/wire"
)

var ErrNilRunFunc = errors.New("ui run func is required")
var ErrVisualizerStoppedEarly = errors.New("visualizer stopped before run completed")

// RunFunc represents a single runtime execution driven by the coordinator.
// The coordinator injects a wire-enabled context; the runner reads wire from ctx.
type RunFunc func(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error)

// VisualizeFunc represents a visualization loop that consumes runtime events.
// It should read until the channel is closed.
type VisualizeFunc func(ctx context.Context, events <-chan runtimeevents.Event) error

type runOutcome struct {
	result runtime.Result
	err    error
}

// Run coordinates runtime execution and event visualization using wire.
func Run(ctx context.Context, run RunFunc, store contextstore.Context, input runtime.Input, visualize VisualizeFunc) (runtime.Result, error) {
	if run == nil {
		return runtime.Result{}, ErrNilRunFunc
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := wire.New(64)
	defer w.Shutdown()

	eventsCh := make(chan runtimeevents.Event, 32)

	// Bridge wire messages to the events channel for the visualizer.
	// Uses a separate context so buffered messages can be drained even
	// after runCtx is cancelled. The bridge only stops when the wire
	// itself shuts down or bridgeCancel is called.
	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	go func() {
		defer close(eventsCh)
		for {
			msg, err := w.Receive(bridgeCtx)
			if err != nil {
				return
			}
			if eventMsg, ok := msg.(wire.EventMessage); ok {
				select {
				case eventsCh <- eventMsg.Event:
				case <-bridgeCtx.Done():
					return
				}
			}
		}
	}()

	wireCtx := wire.WithCurrent(runCtx, w)

	if visualize == nil {
		result, err := run(wireCtx, store, input)
		bridgeCancel()
		return result, err
	}

	runDone := make(chan runOutcome, 1)
	visualizeDone := make(chan error, 1)

	go func() {
		visualizeDone <- visualize(runCtx, eventsCh)
	}()

	go func() {
		result, err := run(wireCtx, store, input)
		// Shutdown the wire so the bridge goroutine stops waiting for more messages.
		// The bridge will drain any buffered messages first (Receive prioritizes the channel).
		w.Shutdown()
		runDone <- runOutcome{result: result, err: err}
	}()

	select {
	case outcome := <-runDone:
		// Run finished first. Wait for visualizer to consume all events,
		// then stop the bridge and cancel the run context.
		visualizeErr := <-visualizeDone
		bridgeCancel()
		cancel()

		if visualizeErr != nil {
			return outcome.result, fmt.Errorf("visualize runtime events: %w", visualizeErr)
		}

		return outcome.result, outcome.err
	case visualizeErr := <-visualizeDone:
		// Visualizer finished first (error or early stop).
		// Cancel run context so the runner unblocks, then wait for it.
		cancel()
		bridgeCancel()

		outcome := <-runDone
		if visualizeErr != nil {
			return outcome.result, fmt.Errorf("visualize runtime events: %w", visualizeErr)
		}

		return outcome.result, ErrVisualizerStoppedEarly
	case <-ctx.Done():
		cancel()
		bridgeCancel()

		outcome := <-runDone
		visualizeErr := <-visualizeDone
		if visualizeErr != nil && !errors.Is(visualizeErr, context.Canceled) {
			return outcome.result, fmt.Errorf("visualize runtime events: %w", visualizeErr)
		}

		return outcome.result, ctx.Err()
	}
}
