package shell

import (
	"errors"
	"io"
)

var ErrLineReadAborted = errors.New("shell line read aborted")

type lineEditor interface {
	ReadLine(prompt string) (string, error)
	AppendHistory(entry string)
	Close() error
}

type lineEditorFactory func(input io.Reader, output io.Writer, history []string) (lineEditor, error)
