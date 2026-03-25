package shell

import (
	"context"
	"testing"
	"time"

	runtimeevents "fimi-cli/internal/runtime/events"
)

func TestEventSinkSendsEventsToChannel(t *testing.T) {
	events := make(chan runtimeevents.Event, 1)
	sink := NewEventSink(events)

	ctx := context.Background()
	event := runtimeevents.TextPart{Text: "hello"}

	err := sink.Emit(ctx, event)
	if err != nil {
		t.Fatalf("Emit returned error: %v", err)
	}

	select {
	case got := <-events:
		if got != event {
			t.Fatalf("received event = %v, want %v", got, event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventSinkRespectsContextCancellation(t *testing.T) {
	events := make(chan runtimeevents.Event, 1)
	sink := NewEventSink(events)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := runtimeevents.TextPart{Text: "hello"}
	err := sink.Emit(ctx, event)

	if err != context.Canceled {
		t.Fatalf("Emit error = %v, want %v", err, context.Canceled)
	}
}
