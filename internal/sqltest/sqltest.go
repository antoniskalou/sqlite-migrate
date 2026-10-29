package sqltest

import (
	"database/sql"
	"maps"
	"testing"
	"testing/fstest"
)

func NewFS(files map[string]string) fstest.MapFS {
	fsys := make(fstest.MapFS)
	addFilesToFS(fsys, files)
	return fsys
}

func MergeFS(fsys fstest.MapFS, files map[string]string) fstest.MapFS {
	result := make(fstest.MapFS)
	maps.Copy(result, fsys)
	addFilesToFS(result, files)
	return result
}

func addFilesToFS(fsys fstest.MapFS, files map[string]string) {
	for name, content := range files {
		fsys[name] = &fstest.MapFile{
			Data: []byte(content),
		}
	}
}

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
