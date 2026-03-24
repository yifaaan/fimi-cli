package ui

import (
	"context"
	"errors"
	"fmt"

	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
)

var ErrNilRunFunc = errors.New("ui run func is required")
var ErrVisualizerStoppedEarly = errors.New("visualizer stopped before run completed")

const defaultEventBufferSize = 32

// RunFunc 表示 coordinator 需要驱动的一次 runtime 执行。
// coordinator 只负责把事件 sink 注入进去，不关心 runtime 的具体装配过程。
type RunFunc func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error)

// VisualizeFunc 表示一个持续消费 runtime 事件流的可视化循环。
// 它应该一直读到 channel 被关闭为止。
type VisualizeFunc func(ctx context.Context, events <-chan runtimeevents.Event) error

type runOutcome struct {
	result runtime.Result
	err    error
}

// Run 协调 runtime 执行和事件消费循环。
func Run(ctx context.Context, run RunFunc, visualize VisualizeFunc) (runtime.Result, error) {
	if run == nil {
		return runtime.Result{}, ErrNilRunFunc
	}
	if visualize == nil {
		return run(ctx, runtimeevents.NoopSink{})
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventsCh := make(chan runtimeevents.Event, defaultEventBufferSize)
	sink := runtimeevents.SinkFunc(func(ctx context.Context, event runtimeevents.Event) error {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case eventsCh <- event:
			return nil
		}
	})

	runDone := make(chan runOutcome, 1)
	visualizeDone := make(chan error, 1)

	go func() {
		visualizeDone <- visualize(runCtx, eventsCh)
	}()

	go func() {
		defer close(eventsCh)

		result, err := run(runCtx, sink)
		runDone <- runOutcome{result: result, err: err}
	}()

	select {
	case outcome := <-runDone:
		cancel()

		visualizeErr := <-visualizeDone
		if visualizeErr != nil {
			return outcome.result, fmt.Errorf("visualize runtime events: %w", visualizeErr)
		}

		return outcome.result, outcome.err
	case visualizeErr := <-visualizeDone:
		cancel()

		outcome := <-runDone
		if visualizeErr != nil {
			return outcome.result, fmt.Errorf("visualize runtime events: %w", visualizeErr)
		}

		return outcome.result, ErrVisualizerStoppedEarly
	case <-ctx.Done():
		cancel()

		outcome := <-runDone
		visualizeErr := <-visualizeDone
		if visualizeErr != nil && !errors.Is(visualizeErr, context.Canceled) {
			return outcome.result, fmt.Errorf("visualize runtime events: %w", visualizeErr)
		}

		return outcome.result, ctx.Err()
	}
}
