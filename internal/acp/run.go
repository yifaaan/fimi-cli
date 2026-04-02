package acp

import (
	"context"
	"errors"
	"fmt"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/wire"
)

var ErrNilWireRunFunc = errors.New("acp wire run func is required")
var ErrWireVisualizerStoppedEarly = errors.New("acp wire visualizer stopped before run completed")

type WireRunFunc func(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error)
type WireVisualizeFunc func(ctx context.Context, messages <-chan wire.Message) error

type runOutcome struct {
	result runtime.Result
	err    error
}

// RunWithWire coordinates runtime execution with a wire-message visualizer.
func RunWithWire(ctx context.Context, run WireRunFunc, store contextstore.Context, input runtime.Input, visualize WireVisualizeFunc) (runtime.Result, error) {
	if run == nil {
		return runtime.Result{}, ErrNilWireRunFunc
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := wire.New(64)
	defer w.Shutdown()

	messagesCh := make(chan wire.Message, 32)

	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	go func() {
		defer close(messagesCh)
		for {
			msg, err := w.Receive(bridgeCtx)
			if err != nil {
				return
			}
			select {
			case messagesCh <- msg:
			case <-bridgeCtx.Done():
				return
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
		visualizeDone <- visualize(runCtx, messagesCh)
	}()

	go func() {
		result, err := run(wireCtx, store, input)
		w.Shutdown()
		runDone <- runOutcome{result: result, err: err}
	}()

	select {
	case outcome := <-runDone:
		visualizeErr := <-visualizeDone
		bridgeCancel()
		cancel()
		if visualizeErr != nil {
			return outcome.result, fmt.Errorf("visualize ACP wire messages: %w", visualizeErr)
		}
		return outcome.result, outcome.err
	case visualizeErr := <-visualizeDone:
		cancel()
		bridgeCancel()
		if visualizeErr != nil {
			return runtime.Result{}, fmt.Errorf("visualize ACP wire messages: %w", visualizeErr)
		}
		return runtime.Result{}, ErrWireVisualizerStoppedEarly
	case <-ctx.Done():
		cancel()
		bridgeCancel()
		outcome := <-runDone
		visualizeErr := <-visualizeDone
		if visualizeErr != nil && !errors.Is(visualizeErr, context.Canceled) {
			return outcome.result, fmt.Errorf("visualize ACP wire messages: %w", visualizeErr)
		}
		return outcome.result, ctx.Err()
	}
}
