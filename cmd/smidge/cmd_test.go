package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/antoniskalou/smidge"
	"github.com/antoniskalou/smidge/internal/sqltest"
)

func TestCmdInit(t *testing.T) {
	t.Parallel()

	ctx, logger := newTestCommandContext(newDefaultFS())
	ctx.rootDir = t.TempDir()

	t.Run("creates migration files", func(t *testing.T) {
		if err := cmdInit(ctx); err != nil {
			t.Fatalf("cmdInit() error = %v", err)
		}

		if info, err := os.Stat(ctx.rootDir); err != nil {
			t.Fatalf("migration directory was not created: %v", err)
		} else if !info.IsDir() {
			t.Fatal("migration path is not a directory")
		}

		for _, name := range []string{
			"001_init.up.sql",
			"001_init.down.sql",
		} {
			path := filepath.Join(ctx.rootDir, name)

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read migration file: %v", err)
			}

			if string(content) == "" {
				t.Error("content is empty")
			}
		}

		assertInitEvent(t, logger, ctx.rootDir)
		assertEmptyEvents(t, logger) // drained
	})

	t.Run("does not overwrite existing files", func(t *testing.T) {
		upPath := filepath.Join(ctx.rootDir, "001_init.up.sql")
		downPath := filepath.Join(ctx.rootDir, "001_init.down.sql")

		originalUp := "-- custom up migration"
		originalDown := "-- custom down migration"

		if err := os.WriteFile(upPath, []byte(originalUp), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(downPath, []byte(originalDown), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := cmdInit(ctx); err != nil {
			t.Fatalf("cmdInit() error = %v", err)
		}

		tests := []struct {
			path string
			want string
		}{
			{path: upPath, want: originalUp},
			{path: downPath, want: originalDown},
		}

		for _, tt := range tests {
			got, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("failed to read %s: %v", tt.path, err)
			}

			if string(got) != tt.want {
				t.Errorf("%s content = %q, want %q",
					filepath.Base(tt.path), string(got), tt.want)
			}
		}

		assertInitEvent(t, logger, ctx.rootDir)
		assertEmptyEvents(t, logger) // drained
	})

	t.Run("creates missing file without overwriting existing one", func(t *testing.T) {
		upPath := filepath.Join(ctx.rootDir, "001_init.up.sql")
		downPath := filepath.Join(ctx.rootDir, "001_init.down.sql")

		originalUp := "-- keep my custom migration"

		if err := os.WriteFile(upPath, []byte(originalUp), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := cmdInit(ctx); err != nil {
			t.Fatalf("cmdInit() error = %v", err)
		}

		gotUp, err := os.ReadFile(upPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotUp) != originalUp {
			t.Errorf("existing migration was overwritten: %q", gotUp)
		}

		_, err = os.ReadFile(downPath)
		if err != nil {
			t.Fatalf("missing down migration was not created: %v", err)
		}

		assertInitEvent(t, logger, ctx.rootDir)
		assertEmptyEvents(t, logger) // drained
	})

	t.Run("returns error when directory cannot be created", func(t *testing.T) {
		blocker := filepath.Join(ctx.rootDir, "file")

		if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx, logger := newTestCommandContext(newDefaultFS())
		ctx.rootDir = blocker

		if err := cmdInit(ctx); err == nil {
			t.Fatal("expected cmdInit() to return an error")
		}

		assertEmptyEvents(t, logger)
	})
}

func TestCmdStatus(t *testing.T) {
	t.Parallel()

	ctx, logger := newTestCommandContext(newDefaultFS())

	t.Run("database open error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.db")

		err := cmdStatus(ctx, path)
		if err == nil {
			t.Fatal("expected an error")
		}

		if !strings.Contains(err.Error(), "failed to open db at "+path) {
			t.Errorf("unexpected error: %v", err)
		}

		assertEmptyEvents(t, logger)
	})

	t.Run("with empty database", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		db, err := connectDB(dbPath, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		if err := cmdStatus(ctx, dbPath); err != nil {
			t.Fatalf("cmdStatus() error = %v", err)
		}

		assertStatusEvent(t, logger, 0, 0, 2)
		assertEmptyEvents(t, logger) // drained
	})

	t.Run("with applied and pending migrations", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		db, err := connectDB(dbPath, false)
		if err != nil {
			t.Fatal(err)
		}

		// apply the first migration, leaving the second pending
		_, err = ctx.migrator.Deploy(db, 1)
		if err != nil {
			db.Close()
			t.Fatalf("Deploy() error = %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if err := cmdStatus(ctx, dbPath); err != nil {
			t.Fatalf("cmdStatus() error = %v", err)
		}

		assertStatusEvent(t, logger, 1, 1, 1)
		assertEmptyEvents(t, logger) // drained
	})
}

func TestCmdDeploy(t *testing.T) {
	t.Parallel()

	ctx, _ := newTestCommandContext(newDefaultFS())

	t.Run("deploy all", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		if err := cmdDeploy(ctx, dbPath, nil); err != nil {
			t.Fatalf("cmdDeploy() error = %v", err)
		}

		// deploy should create the DB if it doesn't exist
		if _, err := os.Stat(dbPath); err != nil {
			t.Fatalf("database was not created: %v", err)
		}

		db, err := connectDB(dbPath, true)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		sqltest.AssertTableExists(t, db, "users", true)
	})

	t.Run("deploy to a target version", func(t *testing.T) {
		target := 1

		dbPath := filepath.Join(t.TempDir(), "test.db")
		if err := cmdDeploy(ctx, dbPath, &target); err != nil {
			t.Fatalf("cmdDeploy() error = %v", err)
		}

		db, err := connectDB(dbPath, true)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		tests := []struct {
			table string
			want  bool
		}{
			{table: "users", want: true},
			{table: "posts", want: false},
		}

		for _, tt := range tests {
			t.Run(tt.table, func(t *testing.T) {
				sqltest.AssertTableExists(t, db, tt.table, tt.want)
			})
		}
	})

	t.Run("migration error", func(t *testing.T) {
		fsys := sqltest.MergeFS(
			newDefaultFS(),
			map[string]string{
				"001_create_users.up.sql":   `this is not valid sql;`,
				"001_create_users.down.sql": `select 1`,
			},
		)

		ctx, _ := newTestCommandContext(fsys)

		dbPath := filepath.Join(t.TempDir(), "test.db")
		err := cmdDeploy(ctx, dbPath, nil)
		if err == nil {
			t.Fatal("expected deployment error")
		}

		if !strings.Contains(err.Error(), "deploy failed:") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestCmdRollback(t *testing.T) {
	t.Parallel()

	ctx, _ := newTestCommandContext(newDefaultFS())

	t.Run("rollback all", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		if err := cmdDeploy(ctx, dbPath, nil); err != nil {
			t.Fatalf("cmdDeploy() error = %v", err)
		}

		// rollback all
		if err := cmdRollback(ctx, dbPath, 0); err != nil {
			t.Fatalf("cmdRollback() error = %v", err)
		}

		db, err := connectDB(dbPath, true)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		sqltest.AssertTableExists(t, db, "users", false)
	})

	t.Run("rollback to a target version", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "test.db")

		if err := cmdDeploy(ctx, dbPath, nil); err != nil {
			t.Fatalf("Deploy() error = %v", err)
		}

		// rollback only the second migration
		if err := cmdRollback(ctx, dbPath, 1); err != nil {
			t.Fatalf("cmdRollback() error = %v", err)
		}

		db, err := connectDB(dbPath, true)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		sqltest.AssertTableExists(t, db, "users", true)
		sqltest.AssertTableExists(t, db, "posts", false)
	})

	t.Run("missing database", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "missing.db")
		err := cmdRollback(ctx, dbPath, 0)
		if err == nil {
			t.Fatal("expected an error")
		}

		if !strings.Contains(err.Error(), "failed to open db at "+dbPath) {
			t.Errorf("unexpected error: %v", err)
		}

		if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
			t.Errorf("database should not have been created; error = %v", err)
		}
	})

	t.Run("migration error", func(t *testing.T) {
		fsys := sqltest.MergeFS(
			newDefaultFS(),
			map[string]string{
				"001_create_users.up.sql": `
				create table users (
					id integer primary key
				);
			`,
				"001_create_users.down.sql": "this is not valid sql;",
			},
		)
		ctx, _ := newTestCommandContext(fsys)

		dbPath := filepath.Join(t.TempDir(), "test.db")
		if err := cmdDeploy(ctx, dbPath, nil); err != nil {
			t.Fatalf("Deploy() error = %v", err)
		}

		err := cmdRollback(ctx, dbPath, 0)
		if err == nil {
			t.Fatal("expected rollback error")
		}

		if !strings.Contains(err.Error(), "rollback failed:") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

var testMigrations = map[string]string{
	"001_create_users.up.sql": `
create table users(
	id integer primary key,
	name text not null
);
`,
	"001_create_users.down.sql": `
drop table users;
`,
	"002_create_posts.up.sql": `
create table posts (
	id integer primary key,
	title text not null
);
`,
	"002_create_posts.down.sql": `
drop table posts;
`,
}

func newDefaultFS() fstest.MapFS {
	return sqltest.NewFS(testMigrations)
}

type logEvent struct {
	kind   string
	path   string
	status migrate.MigrationStatus
}

type spyLogger struct {
	events []logEvent
}

func (l *spyLogger) logInit(path string) {
	l.events = append(l.events, logEvent{
		kind: "init",
		path: path,
	})
}

func (l *spyLogger) logStatus(status migrate.MigrationStatus) {
	l.events = append(l.events, logEvent{
		kind:   "status",
		status: status,
	})
}

func (l *spyLogger) logMigration(event migrate.Event) {
	// we're not capturing migration events for these tests, its already checked
	// by the migrate package tests.
}

func (l *spyLogger) logError(err error) {
	// shouldn't happen
	panic(err)
}

func (l *spyLogger) pop() logEvent {
	end := len(l.events) - 1
	event := l.events[end]
	l.events = l.events[:end]
	return event
}

func newTestCommandContext(fsys fstest.MapFS) (commandContext, *spyLogger) {
	logger := &spyLogger{}
	return commandContext{
		migrator: migrate.New(fsys, nil),
		logger:   logger,
	}, logger
}

func assertInitEvent(t testing.TB, logger *spyLogger, path string) {
	t.Helper()

	event := logger.pop()

	if event.kind != "init" {
		t.Fatalf(`expected event kind "init", got %q`, event.kind)
	}

	if event.path != path {
		t.Errorf("path = %q, want %q", event.path, path)
	}
}

func assertStatusEvent(t testing.TB, logger *spyLogger, version, applied, pending int) {
	t.Helper()

	event := logger.pop()

	if event.kind != "status" {
		t.Fatalf(`expected event kind "status", got %q`, event.kind)
	}

	status := event.status
	if status.Version != version {
		t.Errorf("version = %d, want %d", status.Version, version)
	}

	if len(status.Applied) != applied {
		t.Errorf("applied = %v, want %d migration(s)", status.Applied, applied)
	}

	if len(status.Pending) != pending {
		t.Errorf("pending = %v, want %d migration(s)", status.Pending, pending)
	}
}

func assertEmptyEvents(t testing.TB, logger *spyLogger) {
	t.Helper()

	if len(logger.events) != 0 {
		t.Fatalf("expected empty logger events, got %d", len(logger.events))
	}
}
