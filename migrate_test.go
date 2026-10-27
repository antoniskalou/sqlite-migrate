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

func assertVersion(t *testing.T, m *Migrator, db *sql.DB, want int) {
	t.Helper()

	got, err := m.Version(db)
	if err != nil {
		t.Fatalf("Version(): %v", err)
	}

	if got != want {
		t.Fatalf("Version() = %d, want %d", got, want)
	}
}

func TestMigrationLifecycle(t *testing.T) {
	m, db := newTestMigrator(t)

	type statusAssertion struct {
		version    int
		numApplied int
		numPending int
	}

	assertStatus := func(t *testing.T, want statusAssertion, got MigrationStatus) {
		t.Helper()

		if got.Version != want.version {
			t.Errorf("Version = %d, want %d", got.Version, want.version)
		}

		if len(got.Applied) != want.numApplied {
			t.Errorf("Applied = %d, want %d", len(got.Applied), want.numApplied)
		}

		if len(got.Pending) != want.numPending {
			t.Errorf("Pending = %d, want %d", len(got.Pending), want.numPending)
		}
	}

	t.Run("initial status", func(t *testing.T) {
		status, err := m.Status(db)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, statusAssertion{0, 0, 2}, status)

		want := []string{
			filepath.Join(m.rootDir, "000_t1.up.sql"),
			filepath.Join(m.rootDir, "001_t2.up.sql"),
		}
		if !slices.Equal(status.Pending, want) {
			t.Errorf("Pending = %v, want %v", status.Pending, want)
		}
	})

	t.Run("deploy all", func(t *testing.T) {
		got, err := m.DeployAll(db)
		if err != nil {
			t.Fatal(err)
		}
		if got != 2 {
			t.Fatalf("Deploy() = %d, want 2", got)
		}

		assertVersion(t, m, db, 2)
		sqltest.AssertTableExists(t, db, "t1", true)
		sqltest.AssertTableExists(t, db, "t2", true)

		status, err := m.Status(db)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, statusAssertion{2, 2, 0}, status)
	})

	t.Run("rollback one", func(t *testing.T) {
		got, err := m.Rollback(db, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got != 1 {
			t.Fatalf("Rollback() = %d, want 1", got)
		}

		assertVersion(t, m, db, 1)
		sqltest.AssertTableExists(t, db, "t1", true)
		sqltest.AssertTableExists(t, db, "t2", false)

		status, err := m.Status(db)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, statusAssertion{1, 1, 1}, status)
	})

	t.Run("redeploy", func(t *testing.T) {
		got, err := m.DeployAll(db)
		if err != nil {
			t.Fatal(err)
		}
		if got != 2 {
			t.Fatalf("DeployAll() = %d, want 2", got)
		}

		sqltest.AssertTableExists(t, db, "t1", true)
		sqltest.AssertTableExists(t, db, "t2", true)
	})

	t.Run("rollback all", func(t *testing.T) {
		got, err := m.Rollback(db, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got != 0 {
			t.Fatalf("Rollback() = %d, want 0", got)
		}

		status, err := m.Status(db)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, statusAssertion{0, 0, 2}, status)

		sqltest.AssertTableExists(t, db, "t1", false)
		sqltest.AssertTableExists(t, db, "t2", false)
	})
}

func TestDeployToTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	got, err := m.Deploy(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("Deploy() = %d, want 1", got)
	}

	sqltest.AssertTableExists(t, db, "t1", true)
	sqltest.AssertTableExists(t, db, "t2", false)

	// deploying to the current version does nothing
	got, err = m.Deploy(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("Deploy() at current target = %d, want 1", got)
	}

	sqltest.AssertTableExists(t, db, "t1", true)
	sqltest.AssertTableExists(t, db, "t2", false)
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
	if err == nil {
		t.Fatal("Deploy() succeeded, want migration error")
	}
	if got != 1 {
		t.Errorf("Deploy() = %d, want 1", got)
	}

	assertVersion(t, m, db, 1)
	sqltest.AssertTableExists(t, db, "t1", true)
	sqltest.AssertTableExists(t, db, "t2", false)
}

func TestRollbackTargetValidation(t *testing.T) {
	m, db := newTestMigrator(t)

	if _, err := m.DeployAll(db); err != nil {
		t.Fatal(err)
	}

	for _, target := range []int{-1, 3} {
		t.Run(strings.ReplaceAll(
			strconv.Itoa(target), "-", "minus",
		), func(t *testing.T) {
			got, err := m.Rollback(db, target)
			if err == nil {
				t.Errorf("Rollback(%d) succeeded, want error", target)
			}
			if got != 2 {
				t.Errorf("Rollback(%d) = %d, want 2", target, got)
			}
			assertVersion(t, m, db, 2)
		})
	}
}

func TestMissingMigrationDirectory(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "missing"), nil)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = m.DeployAll(db)
	if err == nil {
		t.Fatal("Deploy() succeeded with missing migration directory")
	}
}

func TestMigrationFailureReturnsError(t *testing.T) {
	m, db := newTestMigrator(t)

	sqltest.WriteMigration(t, m.rootDir, "000_t1.up.sql", `INVALID SQL;`)

	got, err := m.DeployAll(db)
	if err == nil {
		t.Fatal("Deploy() succeeded, want error")
	}
	if got != 0 {
		t.Errorf("Deploy() = %d, want 0", got)
	}

	assertVersion(t, m, db, 0)

	if !strings.Contains(err.Error(), "000_t1.up.sql") {
		t.Errorf("error %q does not mention migration filename", err)
	}
}

func TestRollbackFailurePreservesVersion(t *testing.T) {
	m, db := newTestMigrator(t)

	if _, err := m.DeployAll(db); err != nil {
		t.Fatal(err)
	}

	sqltest.WriteMigration(t, m.rootDir, "001_t2.down.sql", `INVALID SQL;`)

	got, err := m.Rollback(db, 1)
	if err == nil {
		t.Fatal("Rollback() succeeded, want error")
	}
	if got != 2 {
		t.Errorf("Rollback() = %d, want 2", got)
	}

	assertVersion(t, m, db, 2)
	sqltest.AssertTableExists(t, db, "t1", true)
	sqltest.AssertTableExists(t, db, "t2", true)
}

func TestInvalidDeployTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	for _, target := range []int{-1, 3} {
		got, err := m.Deploy(db, target)
		if err == nil {
			t.Errorf("Deploy(%d) succeeded, want error", target)
		}
		if got != 0 {
			t.Errorf("Deploy(%d) = %d, want 0", target, got)
		}
	}
}

func TestDeployPreviousTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	// deploy all first
	version, err := m.DeployAll(db)
	if err != nil {
		t.Fatal(err)
	}

	// current version
	got, err := m.Deploy(db, version)
	if err != nil {
		t.Fatal(err)
	}
	if got != version {
		t.Errorf("Deploy(%d) = %d, want %d", version, got, version)
	}

	// previous version
	target := version - 1
	got, err = m.Deploy(db, target)
	if err != nil {
		t.Fatal(err)
	}
	if got != version {
		t.Errorf("Deploy(%d) = %d, want %d", target, got, version)
	}
}

func TestPreviousRollbackTarget(t *testing.T) {
	m, db := newTestMigrator(t)

	// rollback all first
	version, err := m.Rollback(db, 0)
	if err != nil {
		t.Fatal(err)
	}

	// current version
	got, err := m.Rollback(db, version)
	if err != nil {
		t.Fatal(err)
	}
	if got != version {
		t.Errorf("Deploy(%d) = %d, want %d", version, got, version)
	}

	// next version
	target := version + 1
	got, err = m.Rollback(db, target)
	if err != nil {
		t.Fatal(err)
	}
	if got != version {
		t.Errorf("Deploy(%d) = %d, want %d", target, got, version)
	}
}
