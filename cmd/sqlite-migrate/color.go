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
