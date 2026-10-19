package migrate

import "database/sql"

// TODO: allow setting this
const MigrationDir = "sql"

type DBStatus struct {
	Version int
	Applied []string
	Pending []string
}

func Deploy(db *sql.DB) (int, error) {
	panic("unimplemented")
}

func Rollback(db *sql.DB) (int, error) {
	panic("unimplemented")
}

func Version(db *sql.DB) (int, error) {
	panic("unimplemented")
}

func Status(db *sql.DB) (DBStatus, error) {
	return DBStatus{
		Version: 0,
		Applied: []string{},
		Pending: []string{},
	}, nil
}
