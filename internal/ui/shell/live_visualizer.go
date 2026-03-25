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

	// 这里用 ANSI 回退并清理旧块，让 shell 在普通终端里也能实现最小 liveview。
	if r.renderedLines > 0 {
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
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(r.w, line); err != nil {
			return fmt.Errorf("write shell line: %w", err)
		}
	}

	r.renderedLines = len(lines)
	return nil
}

func visualizeLive(w io.Writer) ui.VisualizeFunc {
	return func(ctx context.Context, events <-chan runtimeevents.Event) error {
		_ = ctx

		renderer := newLiveRenderer(w)
		state := liveState{}

		for event := range events {
			state.Apply(event)
			if err := renderer.Render(state.Lines()); err != nil {
				return err
			}
		}

		return nil
	}
}
