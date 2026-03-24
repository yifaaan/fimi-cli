package ui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"fimi-cli/internal/runtime"
	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestRunStreamsEventsAndReturnsRunResult(t *testing.T) {
	wantResult := runtime.Result{
		Status: runtime.RunStatusFinished,
	}

	gotEvents := make([]runtimeevents.Event, 0, 2)
	result, err := Run(
		context.Background(),
		func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			if err := sink.Emit(ctx, runtimeevents.StepBegin{Number: 1}); err != nil {
				return runtime.Result{}, err
			}
			if err := sink.Emit(ctx, runtimeevents.TextPart{Text: "hello"}); err != nil {
				return runtime.Result{}, err
			}

			return wantResult, nil
		},
		func(ctx context.Context, events <-chan runtimeevents.Event) error {
			for event := range events {
				gotEvents = append(gotEvents, event)
			}

			return nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("Run() result = %#v, want %#v", result, wantResult)
	}

	wantEvents := []runtimeevents.Event{
		runtimeevents.StepBegin{Number: 1},
		runtimeevents.TextPart{Text: "hello"},
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("captured events = %#v, want %#v", gotEvents, wantEvents)
	}
}

func TestRunUsesNoopSinkWhenVisualizerNil(t *testing.T) {
	called := false

	result, err := Run(
		context.Background(),
		func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			called = true
			if err := sink.Emit(ctx, runtimeevents.StepBegin{Number: 1}); err != nil {
				return runtime.Result{}, err
			}

			return runtime.Result{Status: runtime.RunStatusFinished}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !called {
		t.Fatalf("run func called = false, want true")
	}
	if result.Status != runtime.RunStatusFinished {
		t.Fatalf("result.Status = %q, want %q", result.Status, runtime.RunStatusFinished)
	}
}

func TestRunReturnsErrorWhenVisualizerStopsEarly(t *testing.T) {
	result, err := Run(
		context.Background(),
		func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			if err := sink.Emit(ctx, runtimeevents.StepBegin{Number: 1}); err != nil {
				return runtime.Result{Status: runtime.RunStatusInterrupted}, err
			}

			<-ctx.Done()
			return runtime.Result{Status: runtime.RunStatusInterrupted}, ctx.Err()
		},
		func(ctx context.Context, events <-chan runtimeevents.Event) error {
			<-events
			return nil
		},
	)
	if !errors.Is(err, ErrVisualizerStoppedEarly) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, ErrVisualizerStoppedEarly)
	}
	if result.Status != runtime.RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, runtime.RunStatusInterrupted)
	}
}

func TestRunReturnsVisualizerError(t *testing.T) {
	wantErr := errors.New("visualizer failed")

	result, err := Run(
		context.Background(),
		func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			if err := sink.Emit(ctx, runtimeevents.StepBegin{Number: 1}); err != nil {
				return runtime.Result{Status: runtime.RunStatusInterrupted}, err
			}

			<-ctx.Done()
			return runtime.Result{Status: runtime.RunStatusInterrupted}, ctx.Err()
		},
		func(ctx context.Context, events <-chan runtimeevents.Event) error {
			<-events
			return wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if result.Status != runtime.RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, runtime.RunStatusInterrupted)
	}
}

func TestRunReturnsContextCancellation(t *testing.T) {
	rootCtx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
		close(resultCh)
	}()

	result, err := Run(
		rootCtx,
		func(ctx context.Context, sink runtimeevents.Sink) (runtime.Result, error) {
			<-ctx.Done()
			return runtime.Result{Status: runtime.RunStatusInterrupted}, ctx.Err()
		},
		func(ctx context.Context, events <-chan runtimeevents.Event) error {
			<-ctx.Done()
			for range events {
			}
			return ctx.Err()
		},
	)
	<-resultCh

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, context.Canceled)
	}
	if result.Status != runtime.RunStatusInterrupted {
		t.Fatalf("result.Status = %q, want %q", result.Status, runtime.RunStatusInterrupted)
	}
}

func TestRunReturnsErrorForNilRunFunc(t *testing.T) {
	_, err := Run(context.Background(), nil, nil)
	if !errors.Is(err, ErrNilRunFunc) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, ErrNilRunFunc)
	}
}
