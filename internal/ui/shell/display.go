package shell

import (
	"fmt"
	"io"
	"strings"
)

type transcript struct {
	lines []string
}

func (t *transcript) Append(lines []string) {
	if len(lines) == 0 {
		return
	}

	t.lines = append(t.lines, lines...)
}

func (t *transcript) Clear() {
	t.lines = nil
}

func (t transcript) Snapshot() []string {
	return append([]string(nil), t.lines...)
}

type display struct {
	out        io.Writer
	live       *liveRenderer
	transcript transcript
}

func newDisplay(w io.Writer) *display {
	if w == nil {
		w = io.Discard
	}

	return &display{
		out:  w,
		live: newLiveRenderer(w),
	}
}

func (d *display) AppendTranscriptLines(lines []string) error {
	if len(lines) == 0 {
		return nil
	}

	d.transcript.Append(lines)
	return writeTranscript(d.out, lines)
}

func (d *display) AppendTranscriptText(text string) error {
	lines := linesFromText(text)
	return d.AppendTranscriptLines(lines)
}

func (d *display) Clear() error {
	if err := d.live.Clear(); err != nil {
		return err
	}

	d.transcript.Clear()
	if _, err := fmt.Fprint(d.out, clearScreenANSI); err != nil {
		return fmt.Errorf("clear shell display: %w", err)
	}

	return nil
}

func linesFromText(text string) []string {
	trimmed := strings.TrimSuffix(text, "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}
