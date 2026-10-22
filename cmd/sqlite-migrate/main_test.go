package main

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
				db.Close()
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
