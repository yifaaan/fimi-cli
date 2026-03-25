package shell

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func interactiveTTYStatus(input io.Reader, output io.Writer) (bool, string) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false, "TERM=dumb"
	}

	if !isTTYReader(input) {
		return false, "stdin is not a TTY"
	}
	if !isTTYWriter(output) {
		return false, "stdout is not a TTY"
	}

	return true, ""
}

func isTTYReader(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}

	return term.IsTerminal(int(file.Fd()))
}

func isTTYWriter(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}

	return term.IsTerminal(int(file.Fd()))
}
