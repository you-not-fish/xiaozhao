package logger

import (
	"io"
	"os"
)

func writer() io.Writer {
	return os.Stdout
}
