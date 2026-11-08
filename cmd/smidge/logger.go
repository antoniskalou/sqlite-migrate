package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/antoniskalou/smidge"
	"github.com/charmbracelet/colorprofile"
)

type logger interface {
	logInit(path string)
	logStatus(status smidge.MigrationStatus)
	logMigration(event smidge.Event)
	logError(err error)
}

type jsonLogger struct {
	encoder *json.Encoder
}

func newJsonLogger(stdout io.Writer) *jsonLogger {
	return &jsonLogger{
		encoder: json.NewEncoder(stdout),
	}
}

func (l *jsonLogger) logInit(path string) {
	encodeOrPanic(l.encoder, struct{ Path string }{path})
}

func (l *jsonLogger) logStatus(status smidge.MigrationStatus) {
	encodeOrPanic(l.encoder, status)
}

func (l *jsonLogger) logMigration(event smidge.Event) {
	switch event.Kind {
	case smidge.EventApply, smidge.EventSkip, smidge.EventDone:
		encodeOrPanic(l.encoder, event)
	default:
		panicUnhandled(event.Kind)
	}
}

func (l *jsonLogger) logError(err error) {
	encodeOrPanic(l.encoder, struct{ Error string }{err.Error()})
}

func encodeOrPanic(enc *json.Encoder, v any) {
	if err := enc.Encode(v); err != nil {
		// could happen if pipe closes, the filesystem is full, etc.
		// we choose to panic for simplicity, as propagating the error is annoying
		// and would require a change in smidge.OnEventFunc
		panic(err)
	}
}

type prettyLoggerOptions struct {
	verbose bool
	noColor bool
}

type prettyLogger struct {
	stdout io.Writer
	stderr io.Writer
	opts   prettyLoggerOptions
}

func newPrettyLogger(
	stdout io.Writer,
	stderr io.Writer,
	opts prettyLoggerOptions,
) *prettyLogger {
	return &prettyLogger{
		stdout: newOutputWriter(stdout, opts.noColor),
		stderr: newOutputWriter(stderr, opts.noColor),
		opts:   opts,
	}
}

func newOutputWriter(w io.Writer, noColor bool) *colorprofile.Writer {
	writer := colorprofile.NewWriter(w, os.Environ())

	if writer.Profile == colorprofile.NoTTY {
		// skip, if we're in NoTTY we shouldn't change anything
		return writer
	}

	envNoColor := os.Getenv("SMIDGE_NO_COLOR") != ""
	if noColor || envNoColor {
		writer.Profile = colorprofile.ASCII
	}

	return writer
}

func (l *prettyLogger) logInit(path string) {
	fmt.Fprintf(l.stdout, "%s Migration directory initialised at %s\n",
		doneStyle.Render("[DONE]"),
		path,
	)
}

func (l *prettyLogger) logStatus(status smidge.MigrationStatus) {
	applied := status.Applied
	pending := status.Pending

	version := renderLabel(
		"Version:",
		renderVersion(status.Version),
	)

	pendingCount := renderLabel(
		"Pending:",
		pendingStyle.Render(strconv.Itoa(len(pending))),
	)

	output := lipgloss.JoinVertical(
		lipgloss.Top,
		titleStyle.Render("Database status"),
		"",
		version,
		pendingCount,
		"",
		sectionStyle.Render("Applied migrations:"),
		renderList(applied, "✓", upStyle),
		"",
		sectionStyle.Render("Pending migrations:"),
		renderList(pending, "•", pendingStyle),
	)

	fmt.Fprintln(l.stdout, output)
}

func (l *prettyLogger) logMigration(e smidge.Event) {
	var output string

	switch e.Kind {
	case smidge.EventSkip:
		reason := "(already applied)"
		if e.Direction == smidge.Down {
			reason = "(not applied)"
		}

		output = lipgloss.JoinHorizontal(
			lipgloss.Top,
			skipStyle.Render("[SKIP]"),
			" ",
			e.File,
			" ",
			faintStyle.Render(reason),
		)
	case smidge.EventApply:
		dirStyle := upStyle
		if e.Direction == smidge.Down {
			dirStyle = downStyle
		}

		dir := strings.ToUpper(e.Direction.String())

		output = lipgloss.JoinHorizontal(
			lipgloss.Top,
			dirStyle.Render("["+dir+"]"),
			" ",
			e.File,
			" → user_version=",
			renderVersion(e.Version),
		)

		if l.opts.verbose && e.SQL != "" {
			output += "\n" + faintStyle.Render(e.SQL)
		}
	case smidge.EventDone:
		output = lipgloss.JoinHorizontal(
			lipgloss.Top,
			doneStyle.Render("[DONE]"),
			" user_version=",
			renderVersion(e.Version),
		)
	default:
		panicUnhandled(e.Kind)
	}

	fmt.Fprintln(l.stdout, output)
}

func (l *prettyLogger) logError(err error) {
	fmt.Fprintf(l.stderr, "%s %v\n", errorStyle.Render("[ERR]"), err)
}

func panicUnhandled(kind smidge.EventKind) {
	panic(fmt.Sprintf("unhandled event kind: %q", kind))
}
