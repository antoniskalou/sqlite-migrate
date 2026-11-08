package smidge

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBeginImmediate(t *testing.T) {
	t.Run("noop success", func(t *testing.T) {
		db := openTestDB(t)

		tx, err := beginImmediate(context.Background(), db)
		if err != nil {
			t.Fatalf("beginImmediate: %v", err)
		}

		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback: %v", err)
		}
	})

	t.Run("connection acquisition fails", func(t *testing.T) {
		db := openTestDB(t)

		if err := db.Close(); err != nil {
			t.Fatalf("close database: %v", err)
		}

		_, err := beginImmediate(context.Background(), db)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("begin fails when another immediate transaction holds the lock", func(t *testing.T) {
		db := openTestDB(t)

		tx1, err := beginImmediate(context.Background(), db)
		if err != nil {
			t.Fatalf("first beginImmediate: %v", err)
		}
		defer tx1.Rollback()

		_, err = beginImmediate(context.Background(), db)
		if err == nil {
			t.Fatal("expected second BEGIN IMMEDIATE to fail")
		}
	})
}

func TestImmediateTx_Queries(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.Exec(`
		create table users (
			id integer primary key,
			name text not null
		)
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	tx, err := beginImmediate(context.Background(), db)
	if err != nil {
		t.Fatalf("beginImmediate: %v", err)
	}
	defer tx.Rollback()

	t.Run("Exec", func(t *testing.T) {
		_, err := tx.Exec(
			"insert into users (name) values(?)",
			"alice",
		)
		if err != nil {
			t.Fatalf("Exec: %v", err)
		}
	})

	t.Run("QueryRow", func(t *testing.T) {
		var name string
		err := tx.QueryRow(
			"select name from users where id = ?",
			1,
		).Scan(&name)

		if err != nil {
			t.Fatalf("QueryRow: %v", err)
		}

		if name != "alice" {
			t.Fatalf("expected alice, got %q", name)
		}
	})

	t.Run("Query", func(t *testing.T) {
		rows, err := tx.Query("select name from users")
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		defer rows.Close()

		if err := rows.Err(); err != nil {
			t.Fatalf("rows.Err: %v", err)
		}

		if !rows.Next() {
			t.Fatal("expected at least one row, none returned")
		}
	})
}

func TestImmediateTx_Commit(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.Exec(`
		create table users (
			id integer primary key,
			name text not null
		)
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	tx, err := beginImmediate(context.Background(), db)
	if err != nil {
		t.Fatalf("beginImmediate: %v", err)
	}

	if _, err := tx.Exec(
		"insert into users (name) values (?)",
		"alice",
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var name string
	err = db.QueryRow("select name from users where id = 1").Scan(&name)
	if err != nil {
		t.Fatalf("query committed row: %v", err)
	}

	if name != "alice" {
		t.Fatalf("expected alice, got %q", name)
	}

	t.Run("second commit returns ErrTxDone", func(t *testing.T) {
		if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected sql.ErrTxDone, got %v", err)
		}
	})

	t.Run("rollback after commit returns ErrTxDone", func(t *testing.T) {
		if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected sql.ErrTxDone, got %v", err)
		}
	})
}

func TestImmediateTx_Rollback(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.Exec(`
		create table users (
			id integer primary key,
			name text not null
		)
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	tx, err := beginImmediate(context.Background(), db)
	if err != nil {
		t.Fatalf("beginImmediate: %v", err)
	}

	if _, err := tx.Exec(
		"insert into users (name) values (?)",
		"alice",
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := db.QueryRow(
		"select count(*) from users",
	).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", count)
	}

	t.Run("second rollback returns ErrTxDone", func(t *testing.T) {
		if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected sql.ErrTxDone, got %v", err)
		}
	})

	t.Run("commit after rollback returns ErrTxDone", func(t *testing.T) {
		if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected sql.ErrTxDone, got %v", err)
		}
	})
}

func TestImmediateTx_CommitFailureRollsBack(t *testing.T) {
	db := openTestDB(t)

	// setup a test that will failure at commit time rather than insert time
	if _, err := db.Exec(`
		create table parents (
			id integer primary key
		);

		create table children (
			id integer primary key,
			parent_id integer not null,
			foreign key (parent_id)
				references parents(id)
				deferrable initially deferred
		);
	`); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	tx, err := beginImmediate(context.Background(), db)
	if err != nil {
		t.Fatalf("beginImmediate: %v", err)
	}

	if _, err := tx.Exec(`
		insert into children (parent_id) values (999)
	`); err != nil {
		t.Fatalf("insert child: %v", err)
	}

	err = tx.Commit()
	if err == nil {
		t.Fatal("expected commit to fail")
	}

	if !tx.done {
		t.Fatal("expected the transaction to be marked as done")
	}

	var count int
	if err := db.QueryRow(
		"select count(*) from children",
	).Scan(&count); err != nil {
		t.Fatalf("query children: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 children after failed commit and rollback, got %d", count)
	}

	if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("expected sql.ErrTxDone, got %v", err)
	}
}

func TestImmediateTx_RollbackAfterContextCancellation(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.Exec(`
		create table users (
			id integer primary key,
			name text not null
		);
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tx, err := beginImmediate(ctx, db)
	if err != nil {
		cancel()
		t.Fatalf("beginImmediate: %v", err)
	}

	if _, err := tx.Exec(
		"insert into users (name) values (?)",
		"alice",
	); err != nil {
		cancel()
		t.Fatalf("insert: %v", err)
	}

	cancel()

	// rollback must still work despite cancellation
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback after context cancellation: %v", err)
	}

	var count int
	if err := db.QueryRow(
		"select count(*) from users",
	).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", count)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(0)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	db.SetMaxOpenConns(5)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}
