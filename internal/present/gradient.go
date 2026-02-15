package present

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// MakeGradientRamp returns a color ramp of the given length.
func MakeGradientRamp(length int) []lipgloss.Color {
	const startColor = "#F967DC"
	const endColor = "#6B50FF"
	var (
		c        = make([]lipgloss.Color, length)
		start, _ = colorful.Hex(startColor)
		end, _   = colorful.Hex(endColor)
	)
	for i := range length {
		step := start.BlendLuv(end, float64(i)/float64(length))
		c[i] = lipgloss.Color(step.Hex())
	}
	return c
}

// MakeGradientText renders str with a gradient applied rune-by-rune.
func MakeGradientText(baseStyle lipgloss.Style, str string) string {
	const minSize = 3
	if len(str) < minSize {
		return str
	}
	var b strings.Builder
	runes := []rune(str)
	for i, c := range MakeGradientRamp(len(str)) {
		b.WriteString(baseStyle.Foreground(c).Render(string(runes[i])))
	}
	return b.String()
}

// Reverse returns a copy of in in reverse order.
func Reverse[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
