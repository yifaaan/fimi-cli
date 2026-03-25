package shell

import (
	"context"
	"fmt"
	"io"

	runtimeevents "fimi-cli/internal/runtime/events"
	"fimi-cli/internal/ui"
)

const (
	ansiMoveUpFmt = "\033[%dA"
	ansiClearLine = "\r\033[2K"
	ansiMoveDown  = "\033[1B"
)

type liveRenderer struct {
	w             io.Writer
	renderedLines int
}

func newLiveRenderer(w io.Writer) *liveRenderer {
	if w == nil {
		w = io.Discard
	}

	return &liveRenderer{w: w}
}

func (r *liveRenderer) Render(lines []string) error {
	if len(lines) == 0 {
		return nil
	}

	if err := r.Clear(); err != nil {
		return err
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(r.w, line); err != nil {
			return fmt.Errorf("write shell line: %w", err)
		}
	}

	r.renderedLines = len(lines)
	return nil
}

func (r *liveRenderer) Clear() error {
	if r.renderedLines == 0 {
		return nil
	}

	// 这里用 ANSI 回退并清理旧块，让 shell 在普通终端里也能实现最小 liveview。
	if _, err := fmt.Fprintf(r.w, ansiMoveUpFmt, r.renderedLines); err != nil {
		return fmt.Errorf("move shell cursor up: %w", err)
	}

	for i := 0; i < r.renderedLines; i++ {
		if _, err := fmt.Fprint(r.w, ansiClearLine); err != nil {
			return fmt.Errorf("clear shell line: %w", err)
		}
		if i+1 < r.renderedLines {
			if _, err := fmt.Fprint(r.w, ansiMoveDown); err != nil {
				return fmt.Errorf("move shell cursor down: %w", err)
			}
		}
	}

	if r.renderedLines > 1 {
		if _, err := fmt.Fprintf(r.w, ansiMoveUpFmt, r.renderedLines-1); err != nil {
			return fmt.Errorf("restore shell cursor: %w", err)
		}
	}

	r.renderedLines = 0
	return nil
}

func writeTranscript(w io.Writer, lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("write shell transcript line: %w", err)
		}
	}

	return nil
}

func visualizeLive(display *display) ui.VisualizeFunc {
	return func(ctx context.Context, events <-chan runtimeevents.Event) error {
		_ = ctx
		state := liveState{}

		for event := range events {
			if stepBegin, ok := event.(runtimeevents.StepBegin); ok {
				// 当 runtime 进入下一步时，先把上一 step 的 live 内容固化到 transcript。
				// 这样用户在 step 2 期间仍能看到 step 1 的工具活动，而不是屏幕只剩一个 step 标题。
				if stepBegin.Number > 1 {
					if err := flushLiveState(display, state); err != nil {
						return err
					}
				}
			}

			state.Apply(event)
			if err := display.live.Render(state.Lines()); err != nil {
				return err
			}
		}

		return flushLiveState(display, state)
	}
}

func flushLiveState(display *display, state liveState) error {
	finalLines := state.Lines()
	if len(finalLines) == 0 {
		return nil
	}
	if err := display.live.Clear(); err != nil {
		return err
	}
	if err := display.AppendTranscriptLines(finalLines); err != nil {
		return err
	}

	return nil
}
