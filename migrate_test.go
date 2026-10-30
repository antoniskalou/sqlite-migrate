package migrate

import (
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/antoniskalou/sqlite-migrate/internal/sqltest"
	_ "modernc.org/sqlite"
)

const (
	migrationT1Up   = "000_t1.up.sql"
	migrationT1Down = "000_t1.down.sql"
	migrationT2Up   = "001_t2.up.sql"
	migrationT2Down = "001_t2.down.sql"
)

func TestMigrationLifecycle(t *testing.T) {
	t.Parallel()

	spy := &eventSpy{}
	fsys := newDefaultFS()
	m, db := newTestMigrator(t, fsys, spy.OnEvent)

	all := []string{migrationT1Up, migrationT2Up}

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
		assertEvents(t, spy.Drain(), []Event{
			{
				Kind:      EventApply,
				File:      migrationT1Up,
				Version:   1,
				Direction: Up,
				SQL:       string(fsys[migrationT1Up].Data),
			},
			{
				Kind:      EventApply,
				File:      migrationT2Up,
				Version:   2,
				Direction: Up,
				SQL:       string(fsys[migrationT2Up].Data),
			},
			{
				Kind:      EventDone,
				Version:   2,
				Direction: Up,
			},
		})
	})

	t.Run("rollback one", func(t *testing.T) {
		got, err := m.Rollback(db, 1)
		assertResult(t, "Rollback(1)", got, 1, err)

		assertStatus(t, m, db, statusAssertion{
			version: 1,
			applied: []string{migrationT1Up},
			pending: []string{migrationT2Up},
		})
		assertTables(t, db, true, false)
		assertEvents(t, spy.Drain(), []Event{
			{
				Kind:      EventApply,
				File:      migrationT2Down,
				Version:   1,
				Direction: Down,
				SQL:       string(fsys[migrationT2Down].Data),
			},
			{
				Kind:      EventDone,
				Version:   1,
				Direction: Down,
			},
		})
	})

	t.Run("redeploy", func(t *testing.T) {
		got, err := m.DeployAll(db)
		assertResult(t, "DeployAll()", got, 2, err)

		assertStatus(t, m, db, statusAssertion{
			version: 2,
			applied: all,
		})
		assertTables(t, db, true, true)
		assertEvents(t, spy.Drain(), []Event{
			{
				Kind:      EventSkip,
				File:      migrationT1Up,
				Version:   1,
				Direction: Up,
			},
			{
				Kind:      EventApply,
				File:      migrationT2Up,
				Version:   2,
				Direction: Up,
				SQL:       string(fsys[migrationT2Up].Data),
			},
			{
				Kind:      EventDone,
				Version:   2,
				Direction: Up,
			},
		})
	})

	t.Run("rollback all", func(t *testing.T) {
		got, err := m.Rollback(db, 0)
		assertResult(t, "Rollback(0)", got, 0, err)

		assertStatus(t, m, db, statusAssertion{
			version: 0,
			pending: all,
		})
		assertTables(t, db, false, false)
		assertEvents(t, spy.Drain(), []Event{
			{
				Kind:      EventApply,
				File:      migrationT2Down,
				Version:   1,
				Direction: Down,
				SQL:       string(fsys[migrationT2Down].Data),
			},
			{
				Kind:      EventApply,
				File:      migrationT1Down,
				Version:   0,
				Direction: Down,
				SQL:       string(fsys[migrationT1Down].Data),
			},
			{
				Kind:      EventDone,
				Version:   0,
				Direction: Down,
			},
		})
	})

	t.Run("rollback all again", func(t *testing.T) {
		got, err := m.Rollback(db, 0)
		assertResult(t, "Rollback(0)", got, 0, err)
		assertEvents(t, spy.Drain(), []Event{
			{
				Kind:      EventDone,
				Version:   0,
				Direction: Down,
			},
		})
	})
}

func TestDeploy(t *testing.T) {
	t.Parallel()

	all := []string{migrationT1Up, migrationT2Up}

	t.Run("deploy to target", func(t *testing.T) {
		m, db := newTestMigrator(t, newDefaultFS(), nil)

		got, err := m.Deploy(db, 1)
		assertResult(t, "Deploy(1)", got, 1, err)
		assertStatus(t, m, db, statusAssertion{
			version: 1,
			applied: []string{migrationT1Up},
			pending: []string{migrationT2Up},
		})
		assertTables(t, db, true, false)

		// deploying to the current version does nothing
		got, err = m.Deploy(db, 1)
		assertResult(t, "Deploy(1) at current version", got, 1, err)
		assertStatus(t, m, db, statusAssertion{
			version: 1,
			applied: []string{migrationT1Up},
			pending: []string{migrationT2Up},
		})
		assertTables(t, db, true, false)
	})

	t.Run("invalid deploy target", func(t *testing.T) {
		m, db := newTestMigrator(t, newDefaultFS(), nil)

		for _, target := range []int{-1, 3} {
			name := strconv.Itoa(target)
			t.Run(name, func(t *testing.T) {
				got, err := m.Deploy(db, target)
				assertFailure(t, "Deploy("+name+")", got, 0, err)

				assertStatus(t, m, db, statusAssertion{
					version: 0,
					pending: all,
				})
				assertTables(t, db, false, false)
			})
		}
	})

	t.Run("deploy current and previous target does nothing", func(t *testing.T) {
		m, db := newTestMigrator(t, newDefaultFS(), nil)

		// deploy all first
		got, err := m.DeployAll(db)
		assertResult(t, "DeployAll()", got, 2, err)

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
	})

	t.Run("failed deploy does not advance version", func(t *testing.T) {
		fsys := sqltest.MergeFS(
			newDefaultFS(),
			map[string]string{
				migrationT2Up: `invalid sql;`,
			},
		)
		m, db := newTestMigrator(t, fsys, nil)

		got, err := m.DeployAll(db)
		assertFailure(t, "DeployAll()", got, 1, err)
		assertErrorIs(t, err, ErrSQL)

		assertStatus(t, m, db, statusAssertion{
			version: 1,
			applied: []string{migrationT1Up},
			pending: []string{migrationT2Up},
		})
		assertTables(t, db, true, false)
	})
}

func TestRollback(t *testing.T) {
	t.Parallel()

	all := []string{migrationT1Up, migrationT2Up}

	t.Run("invalid rollback target", func(t *testing.T) {
		m, db := newTestMigrator(t, newDefaultFS(), nil)

		got, err := m.DeployAll(db)
		assertResult(t, "DeployAll()", got, 2, err)

		for _, target := range []int{-1, 3} {
			name := strconv.Itoa(target)
			t.Run(name, func(t *testing.T) {
				got, err := m.Rollback(db, target)
				assertFailure(t, "Rollback("+name+")", got, 2, err)

				assertStatus(t, m, db, statusAssertion{
					version: 2,
					applied: all,
				})
				assertTables(t, db, true, true)
			})
		}
	})

	t.Run("rollback previous target does nothing", func(t *testing.T) {
		m, db := newTestMigrator(t, newDefaultFS(), nil)

		got, err := m.DeployAll(db)
		assertResult(t, "DeployAll()", got, 2, err)

		got, err = m.Rollback(db, 2)
		assertResult(t, "Rollback(2)", got, 2, err)

		// a target at or above the current version must not change the state
		assertStatus(t, m, db, statusAssertion{
			version: 2,
			applied: all,
		})
		assertTables(t, db, true, true)
	})

	t.Run("failed rollback does not advance version", func(t *testing.T) {
		fsys := sqltest.MergeFS(
			newDefaultFS(),
			map[string]string{
				migrationT2Down: `invalid sql;`,
			},
		)
		m, db := newTestMigrator(t, fsys, nil)

		got, err := m.DeployAll(db)
		assertResult(t, "DeployAll()", got, 2, err)

		got, err = m.Rollback(db, 1)
		assertFailure(t, "Rollback(1)", got, 2, err)
		assertErrorIs(t, err, ErrSQL)

		assertStatus(t, m, db, statusAssertion{
			version: 2,
			applied: all,
		})
		assertTables(t, db, true, true)
	})
}

func TestVersion(t *testing.T) {
	t.Parallel()

	m, db := newTestMigrator(t, newDefaultFS(), nil)

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

var testMigrations = map[string]string{
	migrationT1Up: `
		create table t1 (
			id integer primary key autoincrement,
			data text not null
		);
	`,
	migrationT1Down: `drop table t1;`,
	migrationT2Up: `
		create table t2 (
			id integer primary key autoincrement,
			data text not null,
			t1_id integer not null references t1(id)
		);
	`,
	migrationT2Down: `drop table t2;`,
}

func newDefaultFS() fstest.MapFS {
	return sqltest.NewFS(testMigrations)
}

func newTestMigrator(t *testing.T, fsys fstest.MapFS, onEvent OnEventFunc) (*Migrator, *sql.DB) {
	t.Helper()

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

	return New(fsys, onEvent), db
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

func assertErrorIs(t *testing.T, got, want error) {
	if !errors.Is(got, want) {
		t.Errorf("expected %v, got %v", got, want)
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

type eventSpy struct {
	events []Event
}

func (s *eventSpy) OnEvent(e Event) {
	s.events = append(s.events, e)
}

func (s *eventSpy) Drain() []Event {
	events := s.events
	s.events = nil
	return events
}

func assertEvents(t *testing.T, got, want []Event) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("events do not match\ngot: %+v\nwant: %+v", got, want)
	}
}
