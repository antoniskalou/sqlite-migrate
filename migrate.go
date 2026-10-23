// Package migrate applies and rolls back SQL migrations for SQLite databases.
//
// Migration state is stored in SQLite's PRAGMA user_version rather than in a
// separate migration-history table. Migration files are selected by their
// direction-specific suffix and processed in filename order.
//
// A migration directory contains .up.sql files for applying migrations and
// (optional) .down.sql files for rolling them back. The current user_version
// represents the number of migrations applied, so existing migration files and
// their ordering must remain consistent with the database's migration state.
package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Version is the current version of the migrate library.
const Version = "0.0.2"

// Migrator configures and executes SQL migrations against a SQLite database.
type Migrator struct {
	// RootDir is the directory containing migration files.
	RootDir string

	// OnEvent, if non-nil, receives events emitted during migration execution.
	OnEvent func(Event)
}

// MigrationStatus describes the migration state inferred from a database's
// user_version and the migration files in the configured directory.
type MigrationStatus struct {
	// Version is the current value of PRAGMA user_version
	Version int

	// Applied contains the paths of migrations considered applied.
	Applied []string

	// Pending contains the paths of migrations not yet applied.
	Pending []string
}

// Deploy applies migrations from the configured directory.
//
// If target is nil, all pending migrations are applied. Otherwise, target
// specifies the desired user_version and must be between zero and the number
// of available up migrations, inclusive.
//
// Deploy returns the resulting user_version. If a migration fails, it returns
// the last successfully applied version (zero if none applied) and the error.
//
// Each migration's SQL statements and the corresponding user_version update
// are executed in a transaction.
func (m *Migrator) Deploy(db *sql.DB, target *int) (int, error) {
	return m.migrate(db, Up, target)
}

// Rollback reverses migrations from the configured directory until the
// specified target version is reached.
//
// Rollback returns the resulting version. If a migration fails, it returns
// the last successfully applied version and the error.
//
// Each rollback migration's SQL statements and the corresponding user_version
// update are executed in a transaction.
func (m *Migrator) Rollback(db *sql.DB, target int) (int, error) {
	return m.migrate(db, Down, &target)
}

// Version returns the database's current SQLite user_version.
func (m *Migrator) Version(db *sql.DB) (int, error) {
	return getUserVersion(db)
}

// Status reports which migrations are considered applied and pending based
// on the database's user_version and the migration files in RootDir.
func (m *Migrator) Status(db *sql.DB) (*MigrationStatus, error) {
	version, err := getUserVersion(db)
	if err != nil {
		return nil, err
	}

	migrations, err := migrations(m.RootDir, Up)
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

func (m *Migrator) emit(event Event) {
	if m.OnEvent != nil {
		m.OnEvent(event)
	}
}

func (m *Migrator) migrate(
	db *sql.DB,
	direction Direction,
	target *int,
) (int, error) {
	version, err := getUserVersion(db)
	if err != nil {
		return 0, err
	}

	files, err := migrations(m.RootDir, direction)
	if err != nil {
		return version, err
	}

	if !isTargetInRange(target, len(files)) {
		return version, fmt.Errorf("invalid target version: %d", *target)
	}

	if direction == Down {
		// we iterate in reverse order for "down" migrations
		slices.Reverse(files)
	}

	for i, path := range files {
		if target != nil {
			if (direction == Up && version >= *target) ||
				(direction == Down && version <= *target) {
				break
			}
		}

		if (direction == Up && i < version) ||
			(direction == Down && len(files)-i != version) {
			m.emit(Event{
				Kind: EventSkip,
				File: path,
			})
			continue
		}

		nextVersion := version + direction.delta()

		m.emit(Event{
			Kind:      EventApply,
			File:      path,
			Direction: direction,
			Version:   nextVersion,
		})

		if err := m.applyMigration(db, path, nextVersion); err != nil {
			return version, err
		}
		version = nextVersion
	}

	m.emit(Event{
		Kind:    EventDone,
		Version: version,
	})
	return version, nil
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

	if err := m.runSQLFile(tx, path); err != nil {
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

func isTargetInRange(target *int, numMigrations int) bool {
	return target == nil || (*target >= 0 && *target <= numMigrations)
}

// interface that allows us to pass a sql.DB or a sql.Tx to functions
type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (m *Migrator) runSQLFile(db sqlExecutor, file string) error {
	sqlBytes, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read SQL file %s: %w", file, err)
	}

	m.emit(Event{
		Kind: EventSQL,
		SQL:  string(sqlBytes),
	})

	if _, err := db.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("execute SQL file %s: %w", file, err)
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

func setUserVersion(db sqlExecutor, version int) error {
	// prepared statements don't work for pragma
	_, err := db.Exec("pragma user_version = " + strconv.Itoa(version))
	return err
}

func migrations(root string, direction Direction) ([]string, error) {
	// os.ReadDir sorts by filename
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	suffix := direction.String() + ".sql"

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
