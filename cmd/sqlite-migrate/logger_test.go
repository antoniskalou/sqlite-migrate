package main

import (
	"bytes"
	"encoding/json"
	"testing"

	migrate "github.com/antoniskalou/sqlite-migrate"
)

func TestPrettyLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   event
		verbose bool
		stdout  string
		stderr  string
	}{
		{
			name: "init",
			event: event{
				Kind: eventInit,
				Path: "/some/file/path",
			},
			stderr: "[DONE] Migration directory initialised at /some/file/path\n",
		},
		{
			name: "status",
			event: event{
				Kind: eventStatus,
				Status: migrate.MigrationStatus{
					Version: 1,
					Applied: []string{"001_create_users.up.sql"},
					Pending: []string{"002_create_posts.up.sql"},
				},
			},
			stderr: `Database status

Version:   1
Pending:   1

Applied migrations:
  ✓ 001_create_users.up.sql

Pending migrations:
  • 002_create_posts.up.sql`,
		},
		{
			name: "skip",
			event: event{
				Kind: eventMigration,
				Migration: migrate.Event{
					Kind:      migrate.EventSkip,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
				},
			},
			stderr: "[SKIP] 001_create_users.up.sql (already applied)\n",
		},
		{
			name: "apply up",
			event: event{
				Kind: eventMigration,
				Migration: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
					Version:   1,
				},
			},
			stderr: "[UP] 001_create_users.up.sql → user_version=1\n",
		},
		{
			name: "apply down",
			event: event{
				Kind: eventMigration,
				Migration: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.down.sql",
					Direction: migrate.Down,
					Version:   1,
				},
			},
			stderr: "[DOWN] 001_create_users.down.sql → user_version=1\n",
		},
		{
			name: "SQL shown when verbose",
			event: event{
				Kind: eventMigration,
				Migration: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
					Version:   1,
					SQL:       "create table users (id integer);",
				},
			},
			verbose: true,
			stdout:  "create table users (id integer);\n",
			stderr:  "[UP] 001_create_users.up.sql → user_version=1\n",
		},
		{
			name: "done",
			event: event{
				Kind: eventMigration,
				Migration: migrate.Event{
					Kind:      migrate.EventDone,
					Version:   2,
					Direction: migrate.Up,
				},
			},
			stderr: "[DONE] user_version=2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			prettyLogger(prettyLoggerOptions{
				stdout:  stdout,
				stderr:  stderr,
				verbose: tt.verbose,
				noColor: true,
			})(tt.event)

			assertOutput(t, "stdout", stdout, tt.stdout)
			assertOutput(t, "stderr", stderr, tt.stderr)
		})
	}
}

func TestPrettyLoggerPanicsOnUnknownEvent(t *testing.T) {
	t.Parallel()

	t.Run("invalid migrate.EventKind", func(t *testing.T) {
		defer assertPanic(t, "prettyLogger", `unhandled event kind: "unknown"`)

		prettyLogger(prettyLoggerOptions{}).toMigrationLogger()(
			migrate.Event{
				Kind: migrate.EventKind("unknown"),
			},
		)
	})

	t.Run("invalid eventKind", func(t *testing.T) {
		defer assertPanic(t, "prettyLogger", `unhandled event kind: "unknown"`)
		prettyLogger(prettyLoggerOptions{})(event{
			Kind: eventKind("unknown"),
		})
	})
}

func TestJSONLogger(t *testing.T) {
	t.Parallel()

	expectedStatus := migrate.MigrationStatus{
		Version: 1,
		Applied: []string{"001_create_users.up.sql"},
		Pending: []string{"001_create_posts.up.sql"},
	}

	expectedMigration := migrate.Event{
		Kind:      migrate.EventApply,
		File:      "001_create_users.up.sql",
		Direction: migrate.Up,
		Version:   1,
		SQL:       "create table users (id integer);",
	}

	tests := []struct {
		name  string
		event event
		want  any
	}{
		{
			name: "init",
			event: event{
				Kind: eventInit,
				Path: "/some/file/path",
			},
			want: struct {
				Kind eventKind
				Path string
			}{eventInit, "/some/file/path"},
		},
		{
			name: "status",
			event: event{
				Kind:   eventStatus,
				Status: expectedStatus,
			},
			want: struct {
				Kind   eventKind
				Status migrate.MigrationStatus
			}{eventStatus, expectedStatus},
		},
		{
			name: "migration",
			event: event{
				Kind:      eventMigration,
				Migration: expectedMigration,
			},
			want: expectedMigration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			jsonLogger(stdout)(tt.event)

			want := &bytes.Buffer{}
			if err := json.NewEncoder(want).Encode(tt.want); err != nil {
				t.Fatal(err)
			}

			assertOutput(t, tt.name, stdout, want.String())
		})
	}
}

func TestJSONLoggerPanicsOnUnknownEvent(t *testing.T) {
	defer assertPanic(t, "jsonLogger", `unhandled event kind: "unknown"`)

	stdout := &bytes.Buffer{}
	jsonLogger(stdout)(event{
		Kind: eventKind("unknown"),
	})
}

func assertOutput(t testing.TB, name string, out *bytes.Buffer, want string) {
	t.Helper()

	if got := out.String(); got != want {
		t.Errorf("%s:\ngot: %q\nwant: %q", name, got, want)
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
