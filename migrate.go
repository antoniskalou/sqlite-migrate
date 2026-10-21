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

// A Migrator is manages the initial configuration for the SQLite migrator.
type Migrator struct {
	RootDir string
	OnEvent func(Event)
}

// A MigrationStatus represents the global status of applied migrations within
// a SQLite project.
type MigrationStatus struct {
	Version int
	Applied []string
	Pending []string
}

// Deploy runs migrations in the configured RootDir. It optionally accepts a
// target parameter to specify which "user version" the deployment should
// run until.
//
// A successful rollback returns err == nil. If the process was stopped due to
// failure, the last applied "user version" is returned.
func (m *Migrator) Deploy(db *sql.DB, target *int) (int, error) {
	return m.migrate(db, Up, target)
}

// Rollback reverses the migrations in the configured RootDir. It optionally
// accepts a target parameter to specify which "user version" the deployment
// should run until.
//
// A successful rollback returns err == nil. If the process was stopped due to
// failure, the last applied "user version" is returned.
func (m *Migrator) Rollback(db *sql.DB, target *int) (int, error) {
	return m.migrate(db, Down, target)
}

// Version returns the current "user version" of the database.
func (m *Migrator) Version(db *sql.DB) (int, error) {
	return getUserVersion(db)
}

// Status returns a [MigrationStatus], constructed by querying the DB and
// the RootDir in the filesystem.
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

	if !isValidTarget(target, len(files)) {
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

		nextVersion := version + direction.Delta()

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

func isValidTarget(target *int, numMigrations int) bool {
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
