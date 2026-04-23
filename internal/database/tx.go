package database

import (
	"context"
	"database/sql"
	"fmt"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
)

func withTx(ctx context.Context, conn *sql.DB, queries *db.Queries, operation string, fn func(*db.Queries) error) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s transaction: %w", operation, err)
	}
	defer tx.Rollback()

	if err := fn(queries.WithTx(tx)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s transaction: %w", operation, err)
	}

	return nil
}

func withTxResult[T any](ctx context.Context, conn *sql.DB, queries *db.Queries, operation string, fn func(*db.Queries) (T, error)) (T, error) {
	var zero T
	var result T

	err := withTx(ctx, conn, queries, operation, func(txQueries *db.Queries) error {
		var err error
		result, err = fn(txQueries)
		return err
	})
	if err != nil {
		return zero, err
	}

	return result, nil
}
