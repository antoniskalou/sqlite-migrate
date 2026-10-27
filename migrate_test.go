package migrate

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/antoniskalou/sqlite-migrate/internal/sqltest"
	_ "modernc.org/sqlite"
)

func newTestMigrator(t *testing.T) (*Migrator, *sql.DB) {
	t.Helper()

	root := t.TempDir()

	sqltest.WriteMigration(t, root, "000_t1.up.sql", `
		create table t1 (
			id integer primary key autoincrement,
			data text not null
		);
	`)
	sqltest.WriteMigration(t, root, "000_t1.down.sql", `
		drop table t1;
	`)

	sqltest.WriteMigration(t, root, "001_t2.up.sql", `
		create table t2 (
			id integer primary key autoincrement,
			data text not null,
			t1_id integer not null references t1(id)
		);
	`)
	sqltest.WriteMigration(t, root, "001_t2.down.sql", `
		drop table t2;
	`)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	db.SetMaxOpenConns(1)

	return New(root, nil), db
}

// gives paths relative to the root dir
func migrationPaths(m *Migrator, names ...string) []string {
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(m.RootDir(), name)
	}
	return paths
}

func assertResult(t *testing.T, operation string, got, want int, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s: %v", operation, err)
	}

	if got != want {
		t.Errorf("%s = %d, want %d", operation, got, want)
	}
}

func assertFailure(t *testing.T, operation string, got, want int, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("%s succeeded, want error", operation)
	}
	if got != want {
		t.Errorf("%s result = %d, want %d", operation, got, want)
	}
}

type statusAssertion struct {
	version int
	applied []string
	pending []string
}

func assertStatus(
	t *testing.T,
	m *Migrator,
	db *sql.DB,
	want statusAssertion,
) {
	t.Helper()

	got, err := m.Status(db)
	if err != nil {
		t.Fatalf("Status(): %v", err)
	}

	if got.Version != want.version {
		t.Errorf("Status().Version = %d, want %d", got.Version, want.version)
	}

	if !slices.Equal(got.Applied, want.applied) {
		t.Errorf("Status().Applied = %v, want %v", got.Applied, want.applied)
	}

	if !slices.Equal(got.Pending, want.pending) {
		t.Errorf("Status().Pending = %v, want %v", got.Pending, want.pending)
	}
}

func assertTables(t *testing.T, db *sql.DB, t1, t2 bool) {
	t.Helper()

	sqltest.AssertTableExists(t, db, "t1", t1)
	sqltest.AssertTableExists(t, db, "t2", t2)
}

func TestMigrationLifecycle(t *testing.T) {
	m, db := newTestMigrator(t)

	t1 := migrationPaths(m, "000_t1.up.sql")
	t2 := migrationPaths(m, "001_t2.up.sql")
	all := slices.Concat(t1, t2)

	t.Run("initial status", func(t *testing.T) {
		assertStatus(t, m, db, statusAssertion{
			version: 0,
			pending: all,
		})
		assertTables(t, db, false, false)
	})

	t.Run("deploy all", func(t *testing.T) {
		got, err := m.DeployAll(db)
		assertResult(t, "DeployAll()", got, 2, err)

		assertStatus(t, m, db, statusAssertion{
			version: 2,
			applied: all,
		})
		assertTables(t, db, true, true)
	})

	t.Run("rollback one", func(t *testing.T) {
		got, err := m.Rollback(db, 1)
		assertResult(t, "Rollback(1)", got, 1, err)

		assertStatus(t, m, db, statusAssertion{
			version: 1,
			applied: t1,
			pending: t2,
		})
		assertTables(t, db, true, false)
	})

	t.Run("redeploy", func(t *testing.T) {
		got, err := m.DeployAll(db)
		assertResult(t, "DeployAll()", got, 2, err)

		assertStatus(t, m, db, statusAssertion{
			version: 2,
			applied: all,
		})
		assertTables(t, db, true, true)
	})

	t.Run("rollback all", func(t *testing.T) {
		got, err := m.Rollback(db, 0)
		assertResult(t, "Rollback(0)", got, 0, err)

		assertStatus(t, m, db, statusAssertion{
			version: 0,
			pending: all,
		})
		assertTables(t, db, false, false)
	})
}

func TestDeployToTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	t1 := migrationPaths(m, "000_t1.up.sql")
	t2 := migrationPaths(m, "001_t2.up.sql")

	got, err := m.Deploy(db, 1)
	assertResult(t, "Deploy(1)", got, 1, err)
	assertStatus(t, m, db, statusAssertion{
		version: 1,
		applied: t1,
		pending: t2,
	})
	assertTables(t, db, true, false)

	// deploying to the current version does nothing
	got, err = m.Deploy(db, 1)
	assertResult(t, "Deploy(1) at current version", got, 1, err)
	assertStatus(t, m, db, statusAssertion{
		version: 1,
		applied: t1,
		pending: t2,
	})
	assertTables(t, db, true, false)
}

func TestFailedMigrationDoesNotAdvanceVersion(t *testing.T) {
	m, db := newTestMigrator(t)

	sqltest.WriteMigration(t, m.rootDir, "001_t2.up.sql", `
		create table t2 (
			id integer primary key
		);
		invalid sql;
	`)

	got, err := m.DeployAll(db)
	assertFailure(t, "Deploy()", got, 1, err)

	assertStatus(t, m, db, statusAssertion{
		version: 1,
		applied: migrationPaths(m, "000_t1.up.sql"),
		pending: migrationPaths(m, "001_t2.up.sql"),
	})
	assertTables(t, db, true, false)
}

func TestRollbackTargetValidation(t *testing.T) {
	m, db := newTestMigrator(t)

	got, err := m.DeployAll(db)
	assertResult(t, "DeployAll()", got, 2, err)

	for _, target := range []int{-1, 3} {
		name := strconv.Itoa(target)
		t.Run(name, func(t *testing.T) {
			got, err := m.Rollback(db, target)
			assertFailure(t, "Rollback("+name+")", got, 2, err)

			assertStatus(t, m, db, statusAssertion{
				version: 2,
				applied: migrationPaths(m, "000_t1.up.sql", "001_t2.up.sql"),
			})
			assertTables(t, db, true, true)
		})
	}
}

func TestMissingMigrationDirectory(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "missing"), nil)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("close database: %v", err)
	}
	defer db.Close()

	got, err := m.DeployAll(db)
	if err == nil {
		t.Fatal("DeployAll() succeeded with missing migration directory")
	}
	if got != 0 {
		t.Errorf("DeployAll() = %d, want 0", got)
	}
}

func TestMigrationFailureReturnsError(t *testing.T) {
	m, db := newTestMigrator(t)

	sqltest.WriteMigration(t, m.rootDir, "000_t1.up.sql", `INVALID SQL;`)

	got, err := m.DeployAll(db)
	assertFailure(t, "DeployAll()", got, 0, err)

	if !strings.Contains(err.Error(), "000_t1.up.sql") {
		t.Errorf("error %q does not mention migration filename", err)
	}

	assertStatus(t, m, db, statusAssertion{
		version: 0,
		pending: migrationPaths(m, "000_t1.up.sql", "001_t2.up.sql"),
	})
	assertTables(t, db, false, false)
}

func TestRollbackFailurePreservesVersion(t *testing.T) {
	m, db := newTestMigrator(t)

	got, err := m.DeployAll(db)
	assertResult(t, "DeployAll()", got, 2, err)

	sqltest.WriteMigration(t, m.rootDir, "001_t2.down.sql", `INVALID SQL;`)

	got, err = m.Rollback(db, 1)
	assertFailure(t, "Rollback(1)", got, 2, err)

	assertStatus(t, m, db, statusAssertion{
		version: 2,
		applied: migrationPaths(m, "000_t1.up.sql", "001_t2.up.sql"),
	})
	assertTables(t, db, true, true)
}

func TestInvalidDeployTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	for _, target := range []int{-1, 3} {
		name := strconv.Itoa(target)
		t.Run(name, func(t *testing.T) {
			got, err := m.Deploy(db, target)
			assertFailure(t, "Deploy("+name+")", got, 0, err)

			assertStatus(t, m, db, statusAssertion{
				version: 0,
				pending: migrationPaths(m, "000_t1.up.sql", "001_t2.up.sql"),
			})
			assertTables(t, db, false, false)
		})
	}
}

func TestDeployPreviousTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	// deploy all first
	got, err := m.DeployAll(db)
	assertResult(t, "DeployAll()", got, 2, err)

	all := migrationPaths(m, "000_t1.up.sql", "001_t2.up.sql")
	for _, target := range []int{2, 1} {
		name := strconv.Itoa(target)
		t.Run(name, func(t *testing.T) {
			got, err := m.Deploy(db, target)
			assertResult(t, "Deploy("+name+")", got, 2, err)

			// deploy must not roll back when the target is below the current version
			assertStatus(t, m, db, statusAssertion{
				version: 2,
				applied: all,
			})
			assertTables(t, db, true, true)
		})
	}
}

func TestInvalidRollbackTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	got, err := m.DeployAll(db)
	assertResult(t, "DeployAll()", got, 2, err)

	for _, target := range []int{-1, 3} {
		name := strconv.Itoa(target)
		t.Run(name, func(t *testing.T) {
			got, err := m.Rollback(db, target)
			assertFailure(t, "Rollback("+name+")", got, 2, err)

			assertStatus(t, m, db, statusAssertion{
				version: 2,
				applied: migrationPaths(m, "000_t1.up.sql", "001_t2.up.sql"),
			})
			assertTables(t, db, true, true)
		})
	}
}

func TestPreviousRollbackTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	got, err := m.DeployAll(db)
	assertResult(t, "DeployAll()", got, 2, err)

	t1 := migrationPaths(m, "000_t1.up.sql")
	all := migrationPaths(m, "000_t1.up.sql", "001_t2.up.sql")

	t.Run("current version", func(t *testing.T) {
		got, err := m.Rollback(db, 2)
		assertResult(t, "Rollback(2)", got, 2, err)

		// a target at or above the current version must not change the state
		assertStatus(t, m, db, statusAssertion{
			version: 2,
			applied: all,
		})
		assertTables(t, db, true, true)
	})

	t.Run("previous version", func(t *testing.T) {
		got, err := m.Rollback(db, 1)
		assertResult(t, "Rollback(1)", got, 1, err)

		assertStatus(t, m, db, statusAssertion{
			version: 1,
			applied: t1,
			pending: migrationPaths(m, "001_t2.up.sql"),
		})
		assertTables(t, db, true, false)
	})
}

func TestVersion(t *testing.T) {
	m, db := newTestMigrator(t)

	got, err := m.Version(db)
	assertResult(t, "Version()", got, 0, err)

	for _, target := range []int{1, 2} {
		name := strconv.Itoa(target)
		t.Run(name, func(t *testing.T) {
			got, err = m.Deploy(db, target)
			assertResult(t, "DeployAll()", got, target, err)

			got, err = m.Version(db)
			assertResult(t, "Version()", got, target, err)
		})
	}
}
