package main

import (
	"bytes"
	"encoding/json"
	"testing"

	migrate "github.com/antoniskalou/sqlite-migrate"
)

func TestPrettyLogger(t *testing.T) {
	render := newRenderer(true)

	tests := []struct {
		name    string
		event   migrate.Event
		verbose bool
		stdout  string
		stderr  string
	}{
		{
			name: "skip",
			event: migrate.Event{
				Kind:      migrate.EventSkip,
				File:      "001_create_users.up.sql",
				Direction: migrate.Up,
			},
			stderr: "[SKIP] 001_create_users.up.sql (already applied)\n",
		},
		{
			name: "apply up",
			event: migrate.Event{
				Kind:      migrate.EventApply,
				File:      "001_create_users.up.sql",
				Direction: migrate.Up,
				Version:   1,
			},
			stderr: "[UP] 001_create_users.up.sql → user_version=1\n",
		},
		{
			name: "apply down",
			event: migrate.Event{
				Kind:      migrate.EventApply,
				File:      "001_create_users.down.sql",
				Direction: migrate.Down,
				Version:   1,
			},
			stderr: "[DOWN] 001_create_users.down.sql → user_version=1\n",
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
			stdout:  "create table users (id integer);\n",
			stderr:  "[UP] 001_create_users.up.sql → user_version=1\n",
		},
		{
			name: "done",
			event: migrate.Event{
				Kind:      migrate.EventDone,
				Version:   2,
				Direction: migrate.Up,
			},
			stderr: "[DONE] user_version=2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			prettyLogger(render, stdout, stderr, true)(tt.event)

			assertOutput(t, "stdout", stdout, tt.stdout)
			assertOutput(t, "stderr", stderr, tt.stderr)
		})
	}
}

func TestPrettyLoggerUnknownKindPanics(t *testing.T) {
	render := newRenderer(true)

	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected prettyLogger to panic for an unknown event kind")
		}

		want := `unhandled event kind: "unknown"`
		if got != want {
			t.Errorf("panic = %v, want %q", got, want)
		}
	}()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	prettyLogger(render, stdout, stderr, false)(migrate.Event{
		Kind: migrate.EventKind("unknown"),
	})
}

func TestJSONLogger(t *testing.T) {
	tests := []struct {
		name  string
		event migrate.Event
	}{

		{
			name: "skip",
			event: migrate.Event{
				Kind:      migrate.EventSkip,
				File:      "001_create_users.up.sql",
				Direction: migrate.Up,
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
			jsonLogger(stdout)(tt.event)

			want := &bytes.Buffer{}
			json.NewEncoder(want).Encode(tt.event)
			assertOutput(t, tt.name, stdout, want.String())
		})
	}
}

func assertOutput(t testing.TB, name string, out *bytes.Buffer, want string) {
	t.Helper()

	got := out.String()
	if want != "" && got != want {
		t.Errorf("%s:\ngot: %q\nwant: %q", name, got, want)
	}
}
