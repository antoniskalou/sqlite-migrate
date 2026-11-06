package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Interface that allows us to pass a sql.DB, a sql.Tx or a immediateTx to functions
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// immediateTx represents a transaction with BEGIN IMMEDIATE.
type immediateTx struct {
	ctx  context.Context
	conn *sql.Conn
	done bool
}

func beginImmediate(ctx context.Context, db *sql.DB) (*immediateTx, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open connection: %w", err)
	}

	_, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE")
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("begin immediate transaction: %w", err)
	}

	return &immediateTx{
		ctx:  ctx,
		conn: conn,
	}, nil
}

func (tx *immediateTx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.conn.ExecContext(tx.ctx, query, args...)
}

func (tx *immediateTx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.conn.QueryContext(tx.ctx, query, args...)
}

func (tx *immediateTx) QueryRow(query string, args ...any) *sql.Row {
	return tx.conn.QueryRowContext(tx.ctx, query, args...)
}

func (tx *immediateTx) Commit() error {
	if tx.done {
		return sql.ErrTxDone
	}

	_, err := tx.Exec("COMMIT")
	if err != nil {
		rollbackErr := tx.Rollback()
		return errors.Join(err, rollbackErr)
	}

	tx.done = true
	return tx.conn.Close()
}

func (tx *immediateTx) Rollback() error {
	if tx.done {
		return sql.ErrTxDone
	}

	tx.done = true

	// rollback must not be cancelled mid-way through running
	ctx := context.WithoutCancel(tx.ctx)
	_, err := tx.conn.ExecContext(ctx, "ROLLBACK")
	closeErr := tx.conn.Close()

	return errors.Join(err, closeErr)
}
