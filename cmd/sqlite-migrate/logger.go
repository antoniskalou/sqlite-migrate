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

type eventKind string

const (
	eventInit      eventKind = "init"
	eventStatus    eventKind = "status"
	eventMigration eventKind = "migration"
)

type event struct {
	Kind eventKind

	// CLI-specific data
	Path   string
	Status migrate.MigrationStatus

	// Migration-specific data
	Migration migrate.Event
}

type logger func(event)

func (l logger) toMigrationLogger() migrate.OnEventFunc {
	return func(e migrate.Event) {
		l(event{
			Kind:      eventMigration,
			Migration: e,
		})
	}
}

func jsonLogger(stdout io.Writer) logger {
	encoder := json.NewEncoder(stdout)

	encode := func(enc any) {
		if err := encoder.Encode(enc); err != nil {
			// really really shouldn't happen, likely a programmer error
			panic(err)
		}
	}

	return func(e event) {
		switch e.Kind {
		case eventInit:
			encode(struct {
				Path string
			}{e.Path})
		case eventStatus:
			encode(e.Status)
		case eventMigration:
			encode(e.Migration)
		}
	}
}

func prettyLogger(
	renderer renderer,
	stdout io.Writer,
	stderr io.Writer,
	verbose bool,
) logger {
	return func(e event) {
		switch e.Kind {
		case eventInit:
			printInit(renderer, stderr, e.Path)
		case eventStatus:
			printStatus(renderer, stderr, e.Status)
		case eventMigration:
			printEvent(renderer, stdout, stderr, verbose)(e.Migration)
		}
	}
}

func printInit(renderer renderer, out io.Writer, path string) {
	fmt.Fprintf(out, "%s Migration directory initialized at %s\n",
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

func printEvent(
	renderer renderer,
	stdout io.Writer,
	stderr io.Writer,
	verbose bool,
) func(migrate.Event) {
	return func(e migrate.Event) {
		switch e.Kind {
		case migrate.EventSkip:
			var faintText string
			if e.Direction == migrate.Up {
				faintText = "(already applied)"
			} else {
				faintText = "(never applied)"
			}

			fmt.Fprintf(stderr, "%s %s %s\n",
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
			fmt.Fprintf(stderr, "%s %s → user_version=%s\n",
				renderer(dirStyle, "["+dir+"]"),
				e.File,
				renderer(versionStyle, strconv.Itoa(e.Version)),
			)

			if verbose {
				fmt.Fprintln(stdout, renderer(faintStyle, e.SQL))
			}
		case migrate.EventDone:
			fmt.Fprintf(stderr, "%s user_version=%s\n",
				renderer(doneStyle, "[DONE]"),
				renderer(versionStyle, strconv.Itoa(e.Version)),
			)
		default:
			panic(fmt.Sprintf("unhandled event kind: %q", e.Kind))
		}
	}
}
