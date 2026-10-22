package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Red)
	titleStyle   = lipgloss.NewStyle().Bold(true)
	sectionStyle = lipgloss.NewStyle().Bold(true)
	skipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlue)
	versionStyle = lipgloss.NewStyle().Bold(true)
	upStyle      = lipgloss.NewStyle().Foreground(lipgloss.Green)
	downStyle    = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	faintStyle   = lipgloss.NewStyle().Faint(true)
	pendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
)

type renderer func(lipgloss.Style, ...string) string

func newRenderer(noColor *bool) renderer {
	return func(style lipgloss.Style, strs ...string) string {
		s := style
		if *noColor {
			return strings.Join(strs, "")
		}
		return s.Render(strs...)
	}
}
