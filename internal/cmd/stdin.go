package cmd

import (
	"io"
	"os"

	"github.com/dotcommander/yai/internal/present"
)

func drainStdin() {
	if present.IsInputTTY() {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}
