package shell

import (
	"context"

	runtimeevents "fimi-cli/internal/runtime/events"
)

// NewEventSink creates an events.Sink that sends events to the provided channel.
// The channel should be buffered to avoid blocking the runtime.
func NewEventSink(events chan<- runtimeevents.Event) runtimeevents.Sink {
	return runtimeevents.SinkFunc(func(ctx context.Context, event runtimeevents.Event) error {
		// Check context first to ensure cancellation is respected
		// even when channel has buffer space available
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case events <- event:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
