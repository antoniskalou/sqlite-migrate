package main

import (
	"os"

	"charm.land/lipgloss/v2"
)

var (
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Red)
	skipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlue)
	versionStyle = lipgloss.NewStyle().Bold(true)
	upStyle      = lipgloss.NewStyle().Foreground(lipgloss.Green)
	downStyle    = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	faintStyle   = lipgloss.NewStyle().Faint(true)
)

type renderer func(lipgloss.Style, ...string) string

func newRenderer(noColor *bool) renderer {
	noColorEnv := os.Getenv("NO_COLOR") != ""

	return func(style lipgloss.Style, strs ...string) string {
		s := style
		if *noColor || noColorEnv {
			s = s.UnsetForeground().UnsetBackground()
		}
		return s.Render(strs...)
	}
}
