package shell

import (
	"bufio"
	"io"
	"strings"
)

type scannerEditor struct {
	scanner *bufio.Scanner
	output  io.Writer
}

func newScannerEditor(input io.Reader, output io.Writer, history []string) (lineEditor, error) {
	if input == nil {
		input = strings.NewReader("")
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	_ = history
	return &scannerEditor{
		scanner: scanner,
		output:  output,
	}, nil
}

func (e *scannerEditor) ReadLine(prompt string) (string, error) {
	if e.output != nil {
		if _, err := io.WriteString(e.output, prompt); err != nil {
			return "", err
		}
	}

	if !e.scanner.Scan() {
		if err := e.scanner.Err(); err != nil {
			return "", err
		}

		return "", io.EOF
	}

	return e.scanner.Text(), nil
}

func (e *scannerEditor) AppendHistory(entry string) {
	_ = entry
}

func (e *scannerEditor) Close() error {
	return nil
}
