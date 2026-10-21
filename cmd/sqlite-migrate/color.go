package main

import "charm.land/lipgloss/v2"

var (
	skipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlue)
	versionStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightGreen).Bold(true)
	upStyle      = lipgloss.NewStyle().Foreground(lipgloss.Green)
	downStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	cyanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	faintStyle   = lipgloss.NewStyle().Faint(true)
)
