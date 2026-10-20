package migrate

import (
	"database/sql"
	"fmt"
	"strconv"
)

// TODO: allow setting this
const MigrationDir = "sql"

type DBStatus struct {
	Version int
	Applied []string
	Pending []string
}

func Deploy(db *sql.DB, target *int) (int, error) {
	if target == nil {
		fmt.Println("deploying all")
	} else {
		fmt.Println("deploying to version", *target)
	}
	panic("unimplemented")
}

func Rollback(db *sql.DB, target *int) (int, error) {
	if target == nil {
		fmt.Println("rolling back all")
	} else {
		fmt.Println("rolling back to version", *target)
	}
	panic("unimplemented")
}

func Version(db *sql.DB) (int, error) {
	return getUserVersion(db)
}

func Status(db *sql.DB) (*DBStatus, error) {
	version, err := getUserVersion(db)
	if err != nil {
		return nil, err
	}

	return &DBStatus{
		Version: version,
		Applied: []string{},
		Pending: []string{},
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
