package shell

import (
	"errors"
	"io"
	"strings"

	"github.com/peterh/liner"
)

type linerEditor struct {
	state *liner.State
}

func newLinerEditor(input io.Reader, output io.Writer, history []string) (lineEditor, error) {
	state := liner.NewLiner()
	state.SetCtrlCAborts(true)
	state.SetTabCompletionStyle(liner.TabPrints)

	for _, entry := range history {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		state.AppendHistory(entry)
	}

	_ = input
	_ = output

	return &linerEditor{state: state}, nil
}

func (e *linerEditor) ReadLine(prompt string) (string, error) {
	line, err := e.state.Prompt(prompt)
	if errors.Is(err, liner.ErrPromptAborted) {
		return "", ErrLineReadAborted
	}

	return line, err
}

func (e *linerEditor) AppendHistory(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}

	e.state.AppendHistory(entry)
}

func (e *linerEditor) Close() error {
	return e.state.Close()
}
