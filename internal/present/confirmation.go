package present

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const defaultAction = "WROTE"

var outputHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1F1F1")).Background(lipgloss.Color("#6C50FF")).Bold(true).Padding(0, 1).MarginRight(1)

// PrintConfirmation prints a short action header plus content.
func PrintConfirmation(action, content string) {
	if action == "" {
		action = defaultAction
	}
	outputHeader = outputHeader.SetString(strings.ToUpper(action))
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Center, outputHeader.String(), content))
}
