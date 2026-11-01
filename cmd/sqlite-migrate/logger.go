package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	migrate "github.com/antoniskalou/sqlite-migrate"
)

type logger interface {
	logInit(path string)
	logStatus(status migrate.MigrationStatus)
	logMigration(event migrate.Event)
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
	render renderer
	stdout io.Writer
	stderr io.Writer
	opts   prettyLoggerOptions
}

func newPrettyLogger(stdout io.Writer, stderr io.Writer, opts prettyLoggerOptions) *prettyLogger {
	return &prettyLogger{
		render: newRenderer(opts.noColor),
		stdout: stdout,
		stderr: stderr,
		opts:   opts,
	}
}

func (l *prettyLogger) logInit(path string) {
	fmt.Fprintf(l.stderr, "%s Migration directory initialised at %s\n",
		l.render(doneStyle, "[DONE]"),
		path,
	)
}

func (l *prettyLogger) logStatus(status migrate.MigrationStatus) {
	applied := status.Applied
	pending := status.Pending

	lines := []string{
		l.render(titleStyle, "Database status"),
		"",
		fmt.Sprintf("%-10s %s", "Version:",
			l.render(versionStyle, strconv.Itoa(status.Version))),
		fmt.Sprintf("%-10s %s", "Pending:",
			l.render(pendingStyle, strconv.Itoa(len(pending)))),
		"",
		l.render(sectionStyle, "Applied migrations:"),
	}

	if len(applied) == 0 {
		lines = append(lines, l.render(faintStyle, "  (none)"))
	} else {
		for _, migration := range applied {
			lines = append(
				lines,
				l.render(upStyle, "  ✓ ")+filepath.Base(migration),
			)
		}
	}

	lines = append(lines, "", l.render(sectionStyle, "Pending migrations:"))

	if len(pending) == 0 {
		lines = append(lines, l.render(faintStyle, "  (none)"))
	} else {
		for _, migration := range pending {
			lines = append(
				lines,
				l.render(pendingStyle, "  • ")+filepath.Base(migration),
			)
		}
	}

	fmt.Fprint(l.stderr, strings.Join(lines, "\n"))
}

func (l *prettyLogger) logMigration(e migrate.Event) {
	switch e.Kind {
	case migrate.EventSkip:
		var faintText string
		if e.Direction == migrate.Up {
			faintText = "(already applied)"
		} else {
			faintText = "(never applied)"
		}

		fmt.Fprintf(l.stderr, "%s %s %s\n",
			l.render(skipStyle, "[SKIP]"),
			e.File,
			l.render(faintStyle, faintText),
		)
	case migrate.EventApply:
		dirStyle := upStyle
		if e.Direction == migrate.Down {
			dirStyle = downStyle
		}

		dir := strings.ToUpper(e.Direction.String())
		fmt.Fprintf(l.stderr, "%s %s → user_version=%s\n",
			l.render(dirStyle, "["+dir+"]"),
			e.File,
			l.render(versionStyle, strconv.Itoa(e.Version)),
		)

		if l.opts.verbose && e.SQL != "" {
			fmt.Fprintln(l.stdout, l.render(faintStyle, e.SQL))
		}
	case migrate.EventDone:
		fmt.Fprintf(l.stderr, "%s user_version=%s\n",
			l.render(doneStyle, "[DONE]"),
			l.render(versionStyle, strconv.Itoa(e.Version)),
		)
	default:
		panicUnhandled(e.Kind)
	}
}

func panicUnhandled(kind migrate.EventKind) {
	panic(fmt.Sprintf("unhandled event kind: %q", kind))
}
