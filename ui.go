package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// infoBox renders a rounded bordered box.
// headerRows are displayed above a thin divider; dataRows below it.
// Each row is a [2]string{"label", "value"} pair.
func infoBox(borderColor lipgloss.Color, headerRows, dataRows [][]string) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)
	labelStyle := lipgloss.NewStyle().Bold(true).Width(12)
	dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	var lines []string
	for _, row := range headerRows {
		lines = append(lines, labelStyle.Render(row[0])+row[1])
	}
	if len(headerRows) > 0 && len(dataRows) > 0 {
		lines = append(lines, dividerStyle.Render(strings.Repeat("─", 44)))
	}
	for _, row := range dataRows {
		lines = append(lines, labelStyle.Render(row[0])+row[1])
	}
	return border.Render(strings.Join(lines, "\n"))
}
