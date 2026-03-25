package shell

import (
	"io"
	"os"
	"strings"
)

type fileDescriptor interface {
	Fd() uintptr
}

func supportsInteractiveTTY(input io.Reader, output io.Writer) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false
	}

	return isTTYReader(input) && isTTYWriter(output)
}

func isTTYReader(input io.Reader) bool {
	fd, ok := input.(fileDescriptor)
	if !ok {
		return false
	}

	file, ok := input.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0 && fd.Fd() > 0
}

func isTTYWriter(output io.Writer) bool {
	fd, ok := output.(fileDescriptor)
	if !ok {
		return false
	}

	file, ok := output.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0 && fd.Fd() > 0
}
