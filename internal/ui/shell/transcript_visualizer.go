package shell

import (
	"context"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui"
)

func visualizeTranscript(display *display) ui.VisualizeFunc {
	return func(ctx context.Context, events <-chan runtimeevents.Event) error {
		_ = ctx
		state := liveState{}

		for event := range events {
			if stepBegin, ok := event.(runtimeevents.StepBegin); ok && stepBegin.Number > 1 {
				if err := display.AppendTranscriptLines(state.Lines()); err != nil {
					return err
				}
				state = liveState{}
			}

			state.Apply(event)
		}

		return display.AppendTranscriptLines(state.Lines())
	}
}
