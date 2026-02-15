package present

import (
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

var isInputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
})

// IsInputTTY reports whether stdin is a TTY.
func IsInputTTY() bool {
	return isInputTTY()
}

var isOutputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})

// IsOutputTTY reports whether stdout is a TTY.
func IsOutputTTY() bool {
	return isOutputTTY()
}

var stdoutRenderer = sync.OnceValue(func() *lipgloss.Renderer {
	return lipgloss.DefaultRenderer()
})

// StdoutRenderer returns a lipgloss renderer bound to stdout.
func StdoutRenderer() *lipgloss.Renderer {
	return stdoutRenderer()
}

var stdoutStyles = sync.OnceValue(func() Styles {
	return MakeStyles(StdoutRenderer())
})

// StdoutStyles returns shared styles bound to stdout.
func StdoutStyles() Styles {
	return stdoutStyles()
}

var stderrRenderer = sync.OnceValue(func() *lipgloss.Renderer {
	return lipgloss.NewRenderer(os.Stderr, termenv.WithColorCache(true))
})

// StderrRenderer returns a lipgloss renderer bound to stderr.
func StderrRenderer() *lipgloss.Renderer {
	return stderrRenderer()
}

var stderrStyles = sync.OnceValue(func() Styles {
	return MakeStyles(StderrRenderer())
})

// StderrStyles returns shared styles bound to stderr.
func StderrStyles() Styles {
	return stderrStyles()
}
