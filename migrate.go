package migrate

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Migrator struct {
	RootDir string
	Logger  *log.Logger
}

type MigrationStatus struct {
	Version int
	Applied []string
	Pending []string
}

func (m *Migrator) Deploy(db *sql.DB, target *int) (int, error) {
	version, err := getUserVersion(db)
	if err != nil {
		return 0, err
	}

	migrations, err := migrations(m.RootDir, "up")
	if err != nil {
		return 0, err
	}

	for i, path := range migrations {
		if target != nil && version == *target {
			break
		}

		if i < version {
			m.log("%s already applied, skipping", path)
			continue
		}

		nextVersion := version + 1
		if err := m.applyMigration(db, path, nextVersion); err != nil {
			return version, err
		}
		version = nextVersion
	}

	m.log("final version: %d", version)
	return version, nil
}

func (m *Migrator) Rollback(db *sql.DB, target *int) (int, error) {
	version, err := getUserVersion(db)
	if err != nil {
		return 0, err
	}

	migrations, err := migrations(m.RootDir, "down")
	if err != nil {
		return 0, err
	}

	// we iterate through rollbacks backwards
	for i, path := range slices.Backward(migrations) {
		if target != nil && version == *target {
			break
		}

		if i+1 != version {
			m.log("%s already applied, skipping", path)
			continue
		}

		nextVersion := version - 1
		if err := m.applyMigration(db, path, nextVersion); err != nil {
			return version, err
		}
		version = nextVersion
	}

	m.log("final version: %d", version)
	return version, nil
}

func (m *Migrator) Version(db *sql.DB) (int, error) {
	return getUserVersion(db)
}

func (m *Migrator) Status(db *sql.DB) (*MigrationStatus, error) {
	version, err := getUserVersion(db)
	if err != nil {
		return nil, err
	}

	migrations, err := migrations(m.RootDir, "up")
	if err != nil {
		return nil, err
	}

	applied := make([]string, 0)
	pending := make([]string, 0)

	for i, m := range migrations {
		if i < version {
			applied = append(applied, m)
		} else {
			pending = append(pending, m)
		}
	}

	return &MigrationStatus{
		Version: version,
		Applied: applied,
		Pending: pending,
	}, nil
}

func (m *Migrator) log(format string, v ...any) {
	if m.Logger != nil {
		m.Logger.Printf(format, v...)
	}
}

func (m *Migrator) applyMigration(
	db *sql.DB,
	path string,
	version int,
) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if m.Logger != nil {
		m.Logger.Printf("running migration: %s", path)
	}

	if err := runSQLFile(tx, path); err != nil {
		return err
	}

	if err := setUserVersion(tx, version); err != nil {
		return fmt.Errorf("set migration version to %d: %w", version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", path, err)
	}

	return nil
}

func getUserVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("pragma user_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get user version: %w", err)
	}
	return version, nil
}

type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func setUserVersion(db sqlExecutor, version int) error {
	// prepared statements don't work for pragma
	_, err := db.Exec("pragma user_version = " + strconv.Itoa(version))
	return err
}

func runSQLFile(db sqlExecutor, file string) error {
	sqlBytes, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read SQL file %s: %w", file, err)
	}

	if _, err := db.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("execute SQL file %s: %w", file, err)
	}

	return nil
}

func migrations(root string, direction string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	suffix := direction + ".sql"

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if strings.HasSuffix(entry.Name(), suffix) {
			files = append(files, filepath.Join(root, entry.Name()))
		}
	}
	return files, nil
}
