package present

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/glamour"
)

const markdownTabWidth = 4

// RenderMarkdownForTTY renders markdown for terminal output.
//
// It mirrors the TUI's markdown rendering behavior closely enough for headless
// commands (e.g. --show / history show) without requiring Bubble Tea.
func RenderMarkdownForTTY(input string, wordWrap int) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		return "", fmt.Errorf("new markdown renderer: %w", err)
	}

	out, err := r.Render(input)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	out = strings.TrimRightFunc(out, unicode.IsSpace)
	out = strings.ReplaceAll(out, "\t", strings.Repeat(" ", markdownTabWidth))
	return out + "\n", nil
}
