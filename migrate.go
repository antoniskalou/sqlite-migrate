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
//
// Each migration's SQL statements and the corresponding user_version update
// are executed in a transaction.
package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// Version is the current version of the migrate library.
const Version = "0.0.8"

// Sentinel errors returned by the migrate package.
var (
	// ErrFilesystem indicates an error accessing migration files.
	ErrFilesystem = errors.New("migration filesystem error")

	// ErrDatabase indicates an error performing a database operation.
	ErrDatabase = errors.New("migration database error")

	// ErrSQL indicates an error executing a migration's SQL statements.
	ErrSQL = errors.New("migration SQL error")
)

type OnEventFunc func(Event)

// Migrator configures and executes SQL migrations against a SQLite database.
type Migrator struct {
	fsys    fs.FS
	onEvent OnEventFunc
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

// New creates a Migrator that uses the given instance of fs.FS as the
// directory containing migration files.
//
// If onEvent is non-nil, New configures the Migrator to deliver migration
// events to onEvent
func New(fsys fs.FS, onEvent OnEventFunc) *Migrator {
	return &Migrator{fsys, onEvent}
}

// DeployAll applies all pending migrations from the configured directory.
//
// DeployAll returns the resulting user_version. If a migration fails, it
// returns the last successfully applied version and the error.
func (m *Migrator) DeployAll(db *sql.DB) (int, error) {
	return m.migrate(db, Up, nil)
}

// Deploy applies pending migrations until target is reached.
//
// target specifies the desired user_version and must be between zero and the
// number of available up migrations, inclusive.
//
// Deploy returns the resulting user_version. If a migration fails, it returns
// the last successfully applied version and the error.
func (m *Migrator) Deploy(db *sql.DB, target int) (int, error) {
	return m.migrate(db, Up, &target)
}

// Rollback reverses migrations from the configured directory until the
// specified target version is reached.
//
// Rollback returns the resulting version. If a migration fails, it returns
// the last successfully applied version and the error.
func (m *Migrator) Rollback(db *sql.DB, target int) (int, error) {
	return m.migrate(db, Down, &target)
}

// Version returns the database's current SQLite user_version.
func (m *Migrator) Version(db *sql.DB) (int, error) {
	return getUserVersion(db)
}

// Status reports which migrations are considered applied and pending based
// on the database's user_version and the migration files in the provided
// filesystem.
func (m *Migrator) Status(db *sql.DB) (MigrationStatus, error) {
	version, err := getUserVersion(db)
	if err != nil {
		return MigrationStatus{}, err
	}

	migrations, err := migrations(m.fsys, Up)
	if err != nil {
		return MigrationStatus{}, err
	}

	applied := make([]string, 0)
	pending := make([]string, 0)

	for i, migration := range migrations {
		if i < version {
			applied = append(applied, migration)
		} else {
			pending = append(pending, migration)
		}
	}

	return MigrationStatus{
		Version: version,
		Applied: applied,
		Pending: pending,
	}, nil
}

func (m *Migrator) emit(event Event) {
	if m.onEvent != nil {
		m.onEvent(event)
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

	files, err := migrations(m.fsys, direction)
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
				Kind:      EventSkip,
				File:      path,
				Direction: direction,
			})
			continue
		}

		nextVersion := version + direction.delta()

		sql, err := applyMigration(db, m.fsys, path, nextVersion)
		if err != nil {
			return version, fmt.Errorf(
				"%s migration %q (version %d): %w",
				direction,
				path,
				nextVersion,
				err,
			)
		}

		m.emit(Event{
			Kind:      EventApply,
			File:      path,
			Direction: direction,
			Version:   nextVersion,
			SQL:       sql,
		})

		version = nextVersion
	}

	m.emit(Event{
		Kind:    EventDone,
		Version: version,
	})
	return version, nil
}

func applyMigration(
	db *sql.DB,
	fsys fs.FS,
	path string,
	version int,
) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("%w: begin migration %q: %w", ErrDatabase, path, err)
	}
	defer tx.Rollback()

	sql, err := runSQLFile(tx, fsys, path)
	if err != nil {
		return sql, err
	}

	if err := setUserVersion(tx, version); err != nil {
		return sql, err
	}

	if err := tx.Commit(); err != nil {
		return sql, fmt.Errorf("%w: commit migration %s: %w", ErrDatabase, path, err)
	}

	return sql, nil
}

func isTargetInRange(target *int, numMigrations int) bool {
	return target == nil || (*target >= 0 && *target <= numMigrations)
}

// interface that allows us to pass a sql.DB or a sql.Tx to functions
type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func runSQLFile(db sqlExecutor, fsys fs.FS, file string) (string, error) {
	sqlBytes, err := fs.ReadFile(fsys, file)
	if err != nil {
		return "", fmt.Errorf("%w: read %q: %w", ErrFilesystem, file, err)
	}

	sql := string(sqlBytes)
	if _, err := db.Exec(sql); err != nil {
		return sql, fmt.Errorf("%w: execute migration %q: %w", ErrSQL, file, err)
	}

	return sql, nil
}

func getUserVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("pragma user_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("%w: get user_version: %w", ErrDatabase, err)
	}
	return version, nil
}

func setUserVersion(db sqlExecutor, version int) error {
	// prepared statements don't work for pragma
	_, err := db.Exec("pragma user_version = " + strconv.Itoa(version))
	if err != nil {
		return fmt.Errorf("%w: set user_version: %w", ErrDatabase, err)
	}
	return nil
}

func migrations(fsys fs.FS, direction Direction) ([]string, error) {
	// ReadDir sorts by filename
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("%w: read migration directory: %w", ErrFilesystem, err)
	}

	suffix := direction.String() + ".sql"

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if strings.HasSuffix(entry.Name(), suffix) {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}
