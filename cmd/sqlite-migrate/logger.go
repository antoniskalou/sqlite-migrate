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

type cliEventKind string

const (
	cliEventInit      cliEventKind = "init"
	cliEventStatus    cliEventKind = "status"
	cliEventMigration cliEventKind = "migration"
)

type cliEvent struct {
	Kind cliEventKind

	// CLI-specific data
	Path   string
	Status migrate.MigrationStatus

	// Migration-specific data
	Migration migrate.Event
}

func (e cliEvent) MarshalJSON() ([]byte, error) {
	switch e.Kind {
	case cliEventInit:
		return json.Marshal(struct {
			Kind cliEventKind
			Path string
		}{e.Kind, e.Path})
	case cliEventStatus:
		return json.Marshal(struct {
			Kind   cliEventKind
			Status migrate.MigrationStatus
		}{cliEventStatus, e.Status})
	case cliEventMigration:
		return json.Marshal(e.Migration)
	default:
		panicUnhandled(string(e.Kind))
		return nil, nil // unreachable
	}
}

type logger func(cliEvent)

func (l logger) toMigrationLogger() migrate.OnEventFunc {
	return func(e migrate.Event) {
		l(cliEvent{
			Kind:      cliEventMigration,
			Migration: e,
		})
	}
}

func jsonLogger(stdout io.Writer) logger {
	encoder := json.NewEncoder(stdout)
	return func(e cliEvent) {
		if err := encoder.Encode(e); err != nil {
			// could happen if pipe closes, the filesystem is full, etc.
			// we choose to panic for simplicity, as propagating the error is annoying
			// and would require a change in migrate.OnEventFunc
			panic(err)
		}
	}
}

type prettyLoggerOptions struct {
	stdout  io.Writer
	stderr  io.Writer
	verbose bool
	noColor bool
}

func prettyLogger(opts prettyLoggerOptions) logger {
	renderer := newRenderer(opts.noColor)

	return func(e cliEvent) {
		switch e.Kind {
		case cliEventInit:
			printInit(renderer, opts.stderr, e.Path)
		case cliEventStatus:
			printStatus(renderer, opts.stderr, e.Status)
		case cliEventMigration:
			printEvent(renderer, opts, e.Migration)
		default:
			panicUnhandled(string(e.Kind))
		}
	}
}

func printInit(renderer renderer, out io.Writer, path string) {
	fmt.Fprintf(out, "%s Migration directory initialised at %s\n",
		renderer(doneStyle, "[DONE]"),
		path,
	)
}

func printStatus(renderer renderer, out io.Writer, status migrate.MigrationStatus) {
	applied := status.Applied
	pending := status.Pending

	lines := []string{
		renderer(titleStyle, "Database status"),
		"",
		fmt.Sprintf("%-10s %s", "Version:",
			renderer(versionStyle, strconv.Itoa(status.Version))),
		fmt.Sprintf("%-10s %s", "Pending:",
			renderer(pendingStyle, strconv.Itoa(len(pending)))),
		"",
		renderer(sectionStyle, "Applied migrations:"),
	}

	if len(applied) == 0 {
		lines = append(lines, renderer(faintStyle, "  (none)"))
	} else {
		for _, migration := range applied {
			lines = append(
				lines,
				renderer(upStyle, "  ✓ ")+filepath.Base(migration),
			)
		}
	}

	lines = append(lines, "", renderer(sectionStyle, "Pending migrations:"))

	if len(pending) == 0 {
		lines = append(lines, renderer(faintStyle, "  (none)"))
	} else {
		for _, migration := range pending {
			lines = append(
				lines,
				renderer(pendingStyle, "  • ")+filepath.Base(migration),
			)
		}
	}

	fmt.Fprint(out, strings.Join(lines, "\n"))
}

func printEvent(renderer renderer, opts prettyLoggerOptions, e migrate.Event) {
	switch e.Kind {
	case migrate.EventSkip:
		var faintText string
		if e.Direction == migrate.Up {
			faintText = "(already applied)"
		} else {
			faintText = "(never applied)"
		}

		fmt.Fprintf(opts.stderr, "%s %s %s\n",
			renderer(skipStyle, "[SKIP]"),
			e.File,
			renderer(faintStyle, faintText),
		)
	case migrate.EventApply:
		dirStyle := upStyle
		if e.Direction == migrate.Down {
			dirStyle = downStyle
		}

		dir := strings.ToUpper(e.Direction.String())
		fmt.Fprintf(opts.stderr, "%s %s → user_version=%s\n",
			renderer(dirStyle, "["+dir+"]"),
			e.File,
			renderer(versionStyle, strconv.Itoa(e.Version)),
		)

		if opts.verbose && e.SQL != "" {
			fmt.Fprintln(opts.stdout, renderer(faintStyle, e.SQL))
		}
	case migrate.EventDone:
		fmt.Fprintf(opts.stderr, "%s user_version=%s\n",
			renderer(doneStyle, "[DONE]"),
			renderer(versionStyle, strconv.Itoa(e.Version)),
		)
	default:
		panicUnhandled(string(e.Kind))
	}
}

func panicUnhandled(kind string) {
	panic(fmt.Sprintf("unhandled event kind: %q", kind))
}
