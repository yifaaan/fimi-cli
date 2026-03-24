package events

import (
	"context"
	"errors"
	"testing"
)

func TestEventKinds(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  Kind
	}{
		{
			name:  "step begin",
			event: StepBegin{Number: 1},
			want:  KindStepBegin,
		},
		{
			name:  "step interrupted",
			event: StepInterrupted{},
			want:  KindStepInterrupted,
		},
		{
			name: "status update",
			event: StatusUpdate{
				Status: StatusSnapshot{ContextUsage: 0.25},
			},
			want: KindStatusUpdate,
		},
		{
			name:  "text part",
			event: TextPart{Text: "hello"},
			want:  KindTextPart,
		},
		{
			name: "tool call",
			event: ToolCall{
				ID:       "call-1",
				Name:     "bash",
				Subtitle: "go test",
			},
			want: KindToolCall,
		},
		{
			name: "tool call part",
			event: ToolCallPart{
				ToolCallID: "call-1",
				Delta:      "{\"command\":\"go",
			},
			want: KindToolCallPart,
		},
		{
			name: "tool result",
			event: ToolResult{
				ToolCallID: "call-1",
				ToolName:   "bash",
				Output:     "ok",
			},
			want: KindToolResult,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.Kind(); got != tt.want {
				t.Fatalf("event.Kind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSinkFuncEmitsEvent(t *testing.T) {
	var got Event

	sink := SinkFunc(func(ctx context.Context, event Event) error {
		got = event
		return nil
	})

	event := StepBegin{Number: 2}
	if err := sink.Emit(context.Background(), event); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	if got != event {
		t.Fatalf("captured event = %#v, want %#v", got, event)
	}
}

func TestSinkFuncPropagatesError(t *testing.T) {
	wantErr := errors.New("sink failed")

	sink := SinkFunc(func(ctx context.Context, event Event) error {
		return wantErr
	})

	err := sink.Emit(context.Background(), StepInterrupted{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Emit() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestNilSinkFuncBehavesAsNoop(t *testing.T) {
	var sink SinkFunc

	if err := sink.Emit(context.Background(), TextPart{Text: "hello"}); err != nil {
		t.Fatalf("Emit() error = %v, want nil", err)
	}
}

func TestNoopSinkDropsEvent(t *testing.T) {
	sink := NoopSink{}

	if err := sink.Emit(context.Background(), ToolResult{ToolCallID: "call-1"}); err != nil {
		t.Fatalf("Emit() error = %v, want nil", err)
	}
}
