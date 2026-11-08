package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	migrate "github.com/antoniskalou/sqlite-migrate"
	"go.followtheprocess.codes/snapshot"
)

func TestPrettyLogger(t *testing.T) {
	t.Parallel()

	t.Run("logInit", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		logger := newPrettyLogger(stdout, io.Discard, prettyLoggerOptions{})
		logger.logInit("/some/file/path")

		assertSnapshot(t, stdout.String())
	})

	t.Run("logStatus", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		logger := newPrettyLogger(stdout, io.Discard, prettyLoggerOptions{})
		logger.logStatus(migrate.MigrationStatus{
			Version: 1,
			Applied: []string{"001_create_users.up.sql"},
			Pending: []string{"002_create_posts.up.sql"},
		})

		assertSnapshot(t, stdout.String())
	})

	t.Run("logMigration", func(t *testing.T) {
		tests := []struct {
			name    string
			event   migrate.Event
			verbose bool
		}{
			{
				name: "skip up",
				event: migrate.Event{
					Kind:      migrate.EventSkip,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
				},
			},
			{
				name: "skip down",
				event: migrate.Event{
					Kind:      migrate.EventSkip,
					File:      "001_create_users.down.sql",
					Direction: migrate.Down,
				},
			},
			{
				name: "apply up",
				event: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.up.sql",
					Direction: migrate.Up,
					Version:   1,
				},
			},
			{
				name: "apply down",
				event: migrate.Event{
					Kind:      migrate.EventApply,
					File:      "001_create_users.down.sql",
					Direction: migrate.Down,
					Version:   1,
				},
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
			},
			{
				name: "done",
				event: migrate.Event{
					Kind:      migrate.EventDone,
					Version:   2,
					Direction: migrate.Up,
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stdout := &bytes.Buffer{}
				logger := newPrettyLogger(
					stdout,
					io.Discard,
					prettyLoggerOptions{
						verbose: tt.verbose,
					},
				)
				logger.logMigration(tt.event)

				assertSnapshot(t, stdout.String())
			})
		}
	})

	t.Run("logError", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		logger := newPrettyLogger(stdout, stderr, prettyLoggerOptions{})

		logger.logError(fmt.Errorf("something went wrong"))

		t.Run("stdout", func(t *testing.T) {
			assertSnapshot(t, stdout.String())
		})

		t.Run("stderr", func(t *testing.T) {
			assertSnapshot(t, stderr.String())
		})
	})
}

func TestPrettyLoggerPanicsOnUnknownEvent(t *testing.T) {
	t.Parallel()

	defer assertPanic(t, "prettyLogger", `unhandled event kind: "unknown"`)

	newPrettyLogger(io.Discard, io.Discard, prettyLoggerOptions{}).logMigration(
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

	t.Run("logError", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		newJsonLogger(stdout).logError(
			fmt.Errorf("something went wrong"),
		)

		assertOutput(t, stdout, `{"Error":"something went wrong"}`+"\n")
	})
}

func TestJSONLoggerPanicsOnUnknownMigrationEvent(t *testing.T) {
	defer assertPanic(t, "jsonLogger", `unhandled event kind: "unknown"`)

	stdout := &bytes.Buffer{}
	newJsonLogger(stdout).logMigration(migrate.Event{
		Kind: "unknown",
	})
}

func assertSnapshot(t testing.TB, got string) {
	t.Helper()

	snap := snapshot.New(
		t,
		snapshot.WithFormatter(snapshot.TextFormatter()),
	)

	snap.Snap(got)
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
