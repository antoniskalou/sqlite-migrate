package main

import (
	"strconv"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"
)

const (
	labelWidth  = 10
	indentWidth = 2
)

var (
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Red)
	labelStyle   = lipgloss.NewStyle().Width(labelWidth)
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

func renderLabel(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		labelStyle.Render(label),
		value,
	)
}

func renderVersion(version int) string {
	return versionStyle.Render(strconv.Itoa(version))
}

func renderList(
	items []string,
	enumerator string,
	enumeratorStyle lipgloss.Style,
) string {
	if len(items) == 0 {
		return faintStyle.
			MarginLeft(indentWidth).
			Render("(none)")
	}

	// convert to []any
	anyItems := make([]any, len(items))
	for i, item := range items {
		anyItems[i] = item
	}

	return lipgloss.NewStyle().
		MarginLeft(indentWidth).
		Render(
			list.New(anyItems...).
				Enumerator(func(_ list.Items, _ int) string {
					return enumerator
				}).
				EnumeratorStyle(
					enumeratorStyle.MarginRight(1),
				).
				String(),
		)
}
