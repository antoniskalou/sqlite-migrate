package sqltest

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func AssertTableExists(t *testing.T, db *sql.DB, name string, want bool) {
	t.Helper()

	var count int
	err := db.QueryRow(`
		select count(*)
		from sqlite_master
		where type = 'table' and name = ?
	`, name).Scan(&count)

	if err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}

	got := count == 1
	if got != want {
		t.Errorf("table %s exists = %v, want %v", name, got, want)
	}
}

func WriteMigration(t *testing.T, root, name, content string) {
	t.Helper()

	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write migration %s: %v", name, err)
	}
}
