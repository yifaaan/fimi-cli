package acp

import (
	"context"
	"errors"
	"testing"
	"time"

	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/wire"
)

func TestRunWithWireReturnsWhenVisualizerStopsEarly(t *testing.T) {
	store := contextstore.New(t.TempDir() + "/history.jsonl")
	releaseRun := make(chan struct{})
	resultCh := make(chan error, 1)

	go func() {
		_, err := RunWithWire(
			context.Background(),
			func(ctx context.Context, store contextstore.Context, input runtime.Input) (runtime.Result, error) {
				<-releaseRun
				return runtime.Result{}, nil
			},
			store,
			runtime.Input{},
			func(ctx context.Context, messages <-chan wire.Message) error {
				return nil
			},
		)
		resultCh <- err
	}()

	select {
	case err := <-resultCh:
		if !errors.Is(err, ErrWireVisualizerStoppedEarly) {
			t.Fatalf("RunWithWire() error = %v, want %v", err, ErrWireVisualizerStoppedEarly)
		}
	case <-time.After(200 * time.Millisecond):
		close(releaseRun)
		t.Fatal("RunWithWire() did not return after visualizer stopped early")
	}

	close(releaseRun)
}
