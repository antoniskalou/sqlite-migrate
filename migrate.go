package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Migrator struct {
	RootDir string
}

type DBStatus struct {
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
			fmt.Println(path + " already applied, skipping...")
			continue
		}

		fmt.Println("running migration:", path)
		if err := runSQLFile(db, path); err != nil {
			return 0, err
		}

		// SQL applied
		version++
		if err := setUserVersion(db, version); err != nil {
			return version, fmt.Errorf("migration applied, pragma user_version failed: %w", err)
		}
	}

	fmt.Println("final version:", version)
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
	for i := len(migrations) - 1; i >= 0; i-- {
		if target != nil && version == *target {
			break
		}

		if version <= 0 {
			continue
		}

		path := migrations[i]

		fmt.Println("running migration:", path)
		if err := runSQLFile(db, path); err != nil {
			fmt.Println(path + " already applied, skipping...")
			return version, err
		}

		version--
		if err := setUserVersion(db, version); err != nil {
			return version, fmt.Errorf("migration applied, pragma user_version failed: %w", err)
		}
	}

	fmt.Println("final version:", version)
	return version, nil
}

func (m *Migrator) Version(db *sql.DB) (int, error) {
	return getUserVersion(db)
}

func (m *Migrator) Status(db *sql.DB) (*DBStatus, error) {
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

	return &DBStatus{
		Version: version,
		Applied: applied,
		Pending: pending,
	}, nil
}

func getUserVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("pragma user_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get user version: %w", err)
	}
	return version, nil
}

func setUserVersion(db *sql.DB, version int) error {
	// prepared statements don't work for pragma
	_, err := db.Exec("pragma user_version = " + strconv.Itoa(version))
	return err
}

func runSQLFile(db *sql.DB, file string) error {
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
