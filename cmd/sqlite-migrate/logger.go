package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	migrate "github.com/antoniskalou/sqlite-migrate"
	"github.com/charmbracelet/colorprofile"
)

type logger interface {
	logInit(path string)
	logStatus(status migrate.MigrationStatus)
	logMigration(event migrate.Event)
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

func (l *jsonLogger) logStatus(status migrate.MigrationStatus) {
	encodeOrPanic(l.encoder, status)
}

func (l *jsonLogger) logMigration(event migrate.Event) {
	switch event.Kind {
	case migrate.EventApply, migrate.EventSkip, migrate.EventDone:
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
		// and would require a change in migrate.OnEventFunc
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

	if noColor {
		// NoTTY to disable all colours and formatting.
		//
		// TODO: support ASCII instead (its more no-color compliant), however tests
		// currently break with it
		writer.Profile = colorprofile.NoTTY
	}

	return writer
}

func (l *prettyLogger) logInit(path string) {
	fmt.Fprintf(l.stdout, "%s Migration directory initialised at %s\n",
		doneStyle.Render("[DONE]"),
		path,
	)
}

func (l *prettyLogger) logStatus(status migrate.MigrationStatus) {
	applied := status.Applied
	pending := status.Pending

	lines := []string{
		titleStyle.Render("Database status"),
		"",
		fmt.Sprintf("%-10s %s", "Version:",
			versionStyle.Render(strconv.Itoa(status.Version))),
		fmt.Sprintf("%-10s %s", "Pending:",
			pendingStyle.Render(strconv.Itoa(len(pending)))),
		"",
		sectionStyle.Render("Applied migrations:"),
	}

	if len(applied) == 0 {
		lines = append(lines, faintStyle.Render("  (none)"))
	} else {
		for _, migration := range applied {
			lines = append(
				lines,
				upStyle.Render("  ✓ ")+filepath.Base(migration),
			)
		}
	}

	lines = append(lines, "", sectionStyle.Render("Pending migrations:"))

	if len(pending) == 0 {
		lines = append(lines, faintStyle.Render("  (none)"))
	} else {
		for _, migration := range pending {
			lines = append(
				lines,
				pendingStyle.Render("  • ")+filepath.Base(migration),
			)
		}
	}

	fmt.Fprintln(l.stdout, strings.Join(lines, "\n"))
}

func (l *prettyLogger) logMigration(e migrate.Event) {
	switch e.Kind {
	case migrate.EventSkip:
		var faintText string
		if e.Direction == migrate.Up {
			faintText = "(already applied)"
		} else {
			faintText = "(not applied)"
		}

		fmt.Fprintf(l.stdout, "%s %s %s\n",
			skipStyle.Render("[SKIP]"),
			e.File,
			faintStyle.Render(faintText),
		)
	case migrate.EventApply:
		dirStyle := upStyle
		if e.Direction == migrate.Down {
			dirStyle = downStyle
		}

		dir := strings.ToUpper(e.Direction.String())
		fmt.Fprintf(l.stdout, "%s %s → user_version=%s\n",
			dirStyle.Render("["+dir+"]"),
			e.File,
			versionStyle.Render(strconv.Itoa(e.Version)),
		)

		if l.opts.verbose && e.SQL != "" {
			fmt.Fprintln(l.stdout, faintStyle.Render(e.SQL))
		}
	case migrate.EventDone:
		fmt.Fprintf(l.stdout, "%s user_version=%s\n",
			doneStyle.Render("[DONE]"),
			versionStyle.Render(strconv.Itoa(e.Version)),
		)
	default:
		panicUnhandled(e.Kind)
	}
}

func (l *prettyLogger) logError(err error) {
	fmt.Fprintf(l.stderr, "%s %v\n", errorStyle.Render("[ERR]"), err)
}

func panicUnhandled(kind migrate.EventKind) {
	panic(fmt.Sprintf("unhandled event kind: %q", kind))
}
