package main

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	migrate "github.com/antoniskalou/sqlite-migrate"
	"github.com/antoniskalou/sqlite-migrate/internal/sqltest"
)

var stdoutMu sync.Mutex

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	// lock Stdout to prevent race condition in parallel tests all capturing
	// from the same Stdout stream
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old

	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	return string(output)
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

// FIXME: duplication with migrate_test.go
func newFS(overrides map[string]string) fstest.MapFS {
	fsys := newDefaultFS()
	for name, content := range overrides {
		fsys[name] = &fstest.MapFile{
			Data: []byte(content),
		}
	}
	return fsys
}

func newTestCommandContext(fsys fstest.MapFS) commandContext {
	return commandContext{
		renderer: newRenderer(true),
		migrator: migrate.New(fsys, nil),
	}
}

func TestWriteIfNotExists(t *testing.T) {
	t.Parallel()

	t.Run("creates a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "migration.sql")
		content := "create table users (id integer);"

		if err := writeIfNotExists(path, content); err != nil {
			t.Fatalf("writeIfNotExists() error = %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		if string(got) != content {
			t.Errorf("content = %q, want %q", got, content)
		}
	})

	t.Run("preserves existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "migration.sql")
		original := "original migration"

		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := writeIfNotExists(path, "replacement"); err != nil {
			t.Fatalf("writeIfNotExists() error = %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != original {
			t.Errorf("existing file was changed: got %q", got)
		}
	})

	t.Run("returns error for invalid parent", func(t *testing.T) {
		path := filepath.Join(
			t.TempDir(), "missing-directory", "migration.sql",
		)

		err := writeIfNotExists(path, "cotnent")
		if err == nil {
			t.Fatal("expected an error for a missing parent directory")
		}
	})

	t.Run("creates empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.sql")

		if err := writeIfNotExists(path, ""); err != nil {
			t.Fatalf("writeIfNotExists() error = %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}

		if info.Size() != 0 {
			t.Errorf("file size = %d, want 0", info.Size())
		}
	})
}

func TestSQLiteDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		options map[string]string
		wantOpt string
		wantVal string
	}{
		{
			name:    "absolute path",
			path:    filepath.Join(t.TempDir(), "test.db"),
			options: map[string]string{},
		},
		{
			name:    "path with spaces",
			path:    filepath.Join(t.TempDir(), "my database.db"),
			options: map[string]string{"mode": "rw"},
			wantOpt: "mode",
			wantVal: "rw",
		},
		{
			name: "multiple options",
			path: filepath.Join(t.TempDir(), "test.db"),
			options: map[string]string{
				"mode": "rw",
				"foo":  "bar",
			},
			wantOpt: "foo",
			wantVal: "bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sqliteDSN(tt.path, tt.options)
			if err != nil {
				t.Fatalf("sqliteDSN() error = %v", err)
			}

			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}

			if u.Scheme != "file" {
				t.Errorf("scheme = %q, want %q", u.Scheme, "file")
			}

			path := u.Path
			if runtime.GOOS == "windows" {
				var hasPrefix bool
				path, hasPrefix = strings.CutPrefix(u.Path, "/")
				if !hasPrefix {
					t.Errorf("%q must have / prefix", u.Path)
				}
			}

			if !filepath.IsAbs(filepath.FromSlash(path)) {
				t.Errorf("path %q is not absolute", path)
			}

			if tt.wantOpt != "" {
				if got := u.Query().Get(tt.wantOpt); got != tt.wantVal {
					t.Errorf(
						"query[%q] = %q, want %q",
						tt.wantOpt, got, tt.wantVal,
					)
				}
			}
		})
	}
}

func TestSQLiteDSNOptionsAreEncoded(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.db")
	dsn, err := sqliteDSN(path, map[string]string{
		"custom": "a b&c",
	})
	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}

	if got := u.Query().Get("custom"); got != "a b&c" {
		t.Errorf("custom option = %q, want %q", got, "a b&c")
	}

	if strings.TrimSpace(u.Path) == "" {
		t.Fatal("DSN path is empty")
	}
}

func TestConnectDB(t *testing.T) {
	t.Parallel()

	t.Run("creates database when allowed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "new.db")

		db, err := connectDB(path, false)
		if err != nil {
			t.Fatalf("connectDB() error = %v", err)
		}
		defer db.Close()

		if _, err := os.Stat(path); err != nil {
			t.Fatalf("database file was not created: %v", err)
		}

		var result int
		if err := db.QueryRow("select 1").Scan(&result); err != nil {
			t.Fatalf("query failed: %v", err)
		}

		if result != 1 {
			t.Errorf("select 1 = %d, want 1", result)
		}
	})

	t.Run("rejects missing database in read-write mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.db")

		db, err := connectDB(path, true)
		if err == nil {
			if db != nil {
				_ = db.Close()
			}
			t.Fatal("expected an error for a missing database")
		}

		if !strings.Contains(err.Error(), "no such file") {
			t.Logf("database error: %v", err)
		}

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("database should not have been created; stat error = %v", err)
		}
	})

	t.Run("opens existing database", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "existing.db")

		db, err := connectDB(path, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		db, err = connectDB(path, true)
		if err != nil {
			t.Fatalf("connectDB() on existing database: %v", err)
		}
		_ = db.Close()
	})
}

func TestCmdInitCreatesMigrationFiles(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(t.TempDir(), "sql")
	ctx := newTestCommandContext(newDefaultFS())
	ctx.rootDir = rootDir

	captureStdout(t, func() {
		if err := cmdInit(ctx); err != nil {
			t.Fatalf("cmdInit() error = %v", err)
		}
	})

	if info, err := os.Stat(rootDir); err != nil {
		t.Fatalf("migration directory was not created: %v", err)
	} else if !info.IsDir() {
		t.Fatal("migration path is not a directory")
	}

	for _, name := range []string{
		"000_init.up.sql",
		"000_init.down.sql",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(rootDir, name)

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read migration file: %v", err)
			}

			if string(content) == "" {
				t.Error("content is empty")
			}
		})
	}
}

func TestCmdInitDoesNotOverwriteExistingFiles(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	upPath := filepath.Join(rootDir, "000_init.up.sql")
	downPath := filepath.Join(rootDir, "000_init.down.sql")

	originalUp := "-- custom up migration"
	originalDown := "-- custom down migration"

	if err := os.WriteFile(upPath, []byte(originalUp), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(downPath, []byte(originalDown), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := newTestCommandContext(newDefaultFS())
	ctx.rootDir = rootDir

	captureStdout(t, func() {
		if err := cmdInit(ctx); err != nil {
			t.Fatalf("cmdInit() error = %v", err)
		}
	})

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
}

func TestCmdInitCreatesMissingFileWithoutOverwritingExistingOne(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	upPath := filepath.Join(rootDir, "000_init.up.sql")
	downPath := filepath.Join(rootDir, "000_init.down.sql")

	originalUp := "-- keep my custom migration"

	if err := os.WriteFile(upPath, []byte(originalUp), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := newTestCommandContext(newDefaultFS())
	ctx.rootDir = rootDir

	captureStdout(t, func() {
		if err := cmdInit(ctx); err != nil {
			t.Fatalf("cmdInit() error = %v", err)
		}
	})

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
}

func TestCmdInitReturnsErrorWhenDirectoryCannotBeCreated(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "file")

	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := newTestCommandContext(newDefaultFS())
	ctx.rootDir = blocker

	captureStdout(t, func() {
		if err := cmdInit(ctx); err == nil {
			t.Fatal("expected cmdInit() to return an error")
		}
	})
}

func TestCmdStatusDatabaseOpenError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.db")
	ctx := newTestCommandContext(newDefaultFS())

	err := cmdStatus(ctx, path)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), "failed to open db at "+path) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCmdStatusEmptyDatabase(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := connectDB(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := newTestCommandContext(newDefaultFS())

	output := captureStdout(t, func() {
		if err := cmdStatus(ctx, dbPath); err != nil {
			t.Fatalf("cmdStatus() error = %v", err)
		}
	})

	for _, want := range []string{
		"Database status",
		"Version:",
		"Pending:",
		"Applied migrations:",
		"Pending migrations:",
		"(none)",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
}

func TestCmdStatusAppliedAndPendingMigrations(t *testing.T) {
	t.Parallel()

	ctx := newTestCommandContext(newDefaultFS())

	dbPath := filepath.Join(t.TempDir(), "test.db")
	// apply the first migration, leaving the second pending
	db, err := connectDB(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ctx.migrator.Deploy(db, 1)
	if err != nil {
		db.Close()
		t.Fatalf("Deploy() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	got := captureStdout(t, func() {
		if err := cmdStatus(ctx, dbPath); err != nil {
			t.Fatalf("cmdStatus() error = %v", err)
		}
	})

	want := `Database status

Version:   1
Pending:   1

Applied migrations:
  ✓ 001_create_users.up.sql

Pending migrations:
  • 002_create_posts.up.sql`

	if got != want {
		t.Errorf("cmdStatus() output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestCmdDeploy(t *testing.T) {
	t.Parallel()

	ctx := newTestCommandContext(newDefaultFS())

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
}

func TestCmdDeployTarget(t *testing.T) {
	t.Parallel()

	ctx := newTestCommandContext(newDefaultFS())
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
}

func TestCmdDeployMigrationError(t *testing.T) {
	t.Parallel()

	fsys := newFS(map[string]string{
		"001_invalid.up.sql":   `this is not valid sql;`,
		"001_invalid.down.sql": `select 1`,
	})

	ctx := newTestCommandContext(fsys)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	err := cmdDeploy(ctx, dbPath, nil)
	if err == nil {
		t.Fatal("expected deployment error")
	}

	if !strings.Contains(err.Error(), "deploy failed:") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCmdRollback(t *testing.T) {
	t.Parallel()

	ctx := newTestCommandContext(newDefaultFS())

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
}

func TestCmdRollbackTarget(t *testing.T) {
	t.Parallel()

	ctx := newTestCommandContext(newDefaultFS())

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
}

func TestCmdRollbackMissingDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.db")
	ctx := newTestCommandContext(newDefaultFS())

	err := cmdRollback(ctx, path, 0)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), "failed to open db at "+path) {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("database should not have been created; error = %v", err)
	}
}

func TestCmdRollbackMigrationError(t *testing.T) {
	t.Parallel()

	fsys := newFS(map[string]string{
		"001_create_users.up.sql": `
			create table users (
				id integer primary key
			);
		`,
		"001_create_users.down.sql": "this is not valid sql;",
	})
	ctx := newTestCommandContext(fsys)

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
}

func TestLogEvent(t *testing.T) {
	render := newRenderer(true)

	tests := []struct {
		name    string
		event   migrate.Event
		verbose bool
		want    string
	}{
		{
			name: "skip",
			event: migrate.Event{
				Kind: migrate.EventSkip,
				File: "001_create_users.up.sql",
			},
			want: "[SKIP] 001_create_users.up.sql (already applied)\n",
		},
		{
			name: "apply up",
			event: migrate.Event{
				Kind:      migrate.EventApply,
				File:      "001_create_users.up.sql",
				Direction: migrate.Up,
				Version:   1,
			},
			want: "[UP] 001_create_users.up.sql → user_version=1\n",
		},
		{
			name: "apply down",
			event: migrate.Event{
				Kind:      migrate.EventApply,
				File:      "001_create_users.down.sql",
				Direction: migrate.Down,
				Version:   1,
			},
			want: "[DOWN] 001_create_users.down.sql → user_version=1\n",
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
			want:    "[UP] 001_create_users.up.sql → user_version=1\ncreate table users (id integer);\n",
		},
		{
			name: "done",
			event: migrate.Event{
				Kind:    migrate.EventDone,
				Version: 2,
			},
			want: "[DONE] user_version=2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := loggerOptions{verbose: tt.verbose}

			got := captureStdout(t, func() {
				logEvent(render, options)(tt.event)
			})

			if got != tt.want {
				t.Errorf("logEvent() output:\ngot: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestLogEventUnknownKindPanics(t *testing.T) {
	render := newRenderer(true)

	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected logEvent to panic for an unknown event kind")
		}

		want := `unhandled event kind: "unknown"`
		if got != want {
			t.Errorf("panic = %v, want %q", got, want)
		}
	}()

	logEvent(render, loggerOptions{})(migrate.Event{
		Kind: migrate.EventKind("unknown"),
	})
}
