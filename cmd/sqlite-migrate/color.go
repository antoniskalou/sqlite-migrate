package main

import "charm.land/lipgloss/v2"

var (
	skipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlue)
	versionStyle = lipgloss.NewStyle().Bold(true)
	upStyle      = lipgloss.NewStyle().Foreground(lipgloss.Green)
	downStyle    = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	faintStyle   = lipgloss.NewStyle().Faint(true)
)

func newRenderer(noColor bool) func(lipgloss.Style, ...string) string {
	return func(style lipgloss.Style, strs ...string) string {
		s := style
		if noColor {
			s = s.UnsetForeground().UnsetBackground()
		}
		return s.Render(strs...)
	}
}
