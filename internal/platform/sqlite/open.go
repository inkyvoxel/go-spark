package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	defaultBusyTimeoutMillis = 5000
	defaultMaxOpenConns      = 1
)

type OpenOptions struct {
	BusyTimeoutMillis int
	MaxOpenConns      int
}

func Open(path string) (*sql.DB, error) {
	return OpenWithOptions(path, OpenOptions{})
}

func OpenWithOptions(path string, opts OpenOptions) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	configureConnectionPool(db, opts)
	if err := applyPragmas(db, opts); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func configureConnectionPool(db *sql.DB, opts OpenOptions) {
	maxOpenConns := opts.MaxOpenConns
	if maxOpenConns == 0 {
		maxOpenConns = defaultMaxOpenConns
	}
	db.SetMaxOpenConns(maxOpenConns)
}

func applyPragmas(db *sql.DB, opts OpenOptions) error {
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	busyTimeoutMillis := opts.BusyTimeoutMillis
	if busyTimeoutMillis == 0 {
		busyTimeoutMillis = defaultBusyTimeoutMillis
	}

	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMillis)); err != nil {
		return fmt.Errorf("set sqlite busy timeout: %w", err)
	}

	return nil
}
