package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	migrate "github.com/antoniskalou/sqlite-migrate"
)

func TestPrettyLogger(t *testing.T) {
	t.Parallel()

	t.Run("logInit", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		logger := newPrettyLogger(
			stdout,
			prettyLoggerOptions{
				verbose: true,
				noColor: true,
			},
		)
		logger.logInit("/some/file/path")

		want := "[DONE] Migration directory initialised at /some/file/path\n"
		assertOutput(t, stdout, want)
	})

	t.Run("logStatus", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		logger := newPrettyLogger(
			stdout,
			prettyLoggerOptions{
				verbose: true,
				noColor: true,
			},
		)
		logger.logStatus(migrate.MigrationStatus{
			Version: 1,
			Applied: []string{"001_create_users.up.sql"},
			Pending: []string{"002_create_posts.up.sql"},
		})

		want := `Database status

Version:   1
Pending:   1

Applied migrations:
  ✓ 001_create_users.up.sql

Pending migrations:
  • 002_create_posts.up.sql`

		assertOutput(t, stdout, want)
	})

	t.Run("logMigration", func(t *testing.T) {
		tests := []struct {
			name    string
			event   migrate.Event
			verbose bool
			stdout  string
		}{
			{
				name: "skip",
				event: migrate.Event{
					Kind:      migrate.EventSkip,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
				},
				stdout: "[SKIP] 001_create_users.up.sql (already applied)\n",
			},
			{
				name: "apply up",
				event: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
					Version:   1,
				},
				stdout: "[UP] 001_create_users.up.sql → user_version=1\n",
			},
			{
				name: "apply down",
				event: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.down.sql",
					Direction: migrate.Down,
					Version:   1,
				},
				stdout: "[DOWN] 001_create_users.down.sql → user_version=1\n",
			},
			{
				name: "SQL shown when verbose",
				event: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
					Version:   1,
					SQL:       "create table users (id integer);",
				},
				verbose: true,
				stdout:  "[UP] 001_create_users.up.sql → user_version=1\ncreate table users (id integer);\n",
			},
			{
				name: "done",
				event: migrate.Event{
					Kind:      migrate.EventDone,
					Version:   2,
					Direction: migrate.Up,
				},
				stdout: "[DONE] user_version=2\n",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stdout := &bytes.Buffer{}
				logger := newPrettyLogger(stdout, prettyLoggerOptions{
					verbose: tt.verbose,
					noColor: true,
				})
				logger.logMigration(tt.event)

				assertOutput(t, stdout, tt.stdout)
			})
		}
	})

}

func TestPrettyLoggerPanicsOnUnknownEvent(t *testing.T) {
	t.Parallel()

	defer assertPanic(t, "prettyLogger", `unhandled event kind: "unknown"`)

	newPrettyLogger(io.Discard, prettyLoggerOptions{}).logMigration(
		migrate.Event{
			Kind: migrate.EventKind("unknown"),
		},
	)
}

func TestJSONLogger(t *testing.T) {
	t.Parallel()

	t.Run("logInit", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		newJsonLogger(stdout).logInit("/some/file/path")

		want := &bytes.Buffer{}
		init := struct{ Path string }{"/some/file/path"}
		if err := json.NewEncoder(want).Encode(init); err != nil {
			t.Fatal(err)
		}
		assertOutput(t, stdout, want.String())
	})

	t.Run("logStatus", func(t *testing.T) {
		expectedStatus := migrate.MigrationStatus{
			Version: 1,
			Applied: []string{"001_create_users.up.sql"},
			Pending: []string{"001_create_posts.up.sql"},
		}

		stdout := &bytes.Buffer{}
		newJsonLogger(stdout).logStatus(expectedStatus)

		want := &bytes.Buffer{}
		if err := json.NewEncoder(want).Encode(expectedStatus); err != nil {
			t.Fatal(err)
		}
		assertOutput(t, stdout, want.String())
	})

	t.Run("logMigration", func(t *testing.T) {
		expectedMigration := migrate.Event{
			Kind:      migrate.EventApply,
			File:      "001_create_users.up.sql",
			Direction: migrate.Up,
			Version:   1,
			SQL:       "create table users (id integer);",
		}

		stdout := &bytes.Buffer{}
		newJsonLogger(stdout).logMigration(expectedMigration)

		want := &bytes.Buffer{}
		if err := json.NewEncoder(want).Encode(expectedMigration); err != nil {
			t.Fatal(err)
		}

		assertOutput(t, stdout, want.String())
	})
}

func TestJSONLoggerPanicsOnUnknownMigrationEvent(t *testing.T) {
	defer assertPanic(t, "jsonLogger", `unhandled event kind: "unknown"`)

	stdout := &bytes.Buffer{}
	newJsonLogger(stdout).logMigration(migrate.Event{
		Kind: "unknown",
	})
}

func assertOutput(t testing.TB, out *bytes.Buffer, want string) {
	t.Helper()

	if got := out.String(); got != want {
		t.Errorf("unexpected output:\ngot: %q\nwant: %q", got, want)
	}
}

func assertPanic(t testing.TB, name string, want string) {
	got := recover()

	if got == nil {
		t.Fatalf("expected %s to panic for an unknown event kind", name)
	}

	if got != want {
		t.Errorf("panic = %v, want %q", got, want)
	}
}
