package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	// DefaultBusyTimeoutMillis gives SQLite time to wait on a locked database
	// before returning SQLITE_BUSY. The starter keeps this small and explicit.
	DefaultBusyTimeoutMillis = 5000
	// DefaultMaxOpenConns keeps the starter on a single writable SQLite
	// connection by default, which matches its single-node operating model.
	DefaultMaxOpenConns = 1
)

// OpenOptions controls SQLite connection tuning for the starter.
//
// The defaults intentionally stay small and conservative:
// - foreign keys are always enabled
// - busy timeout defaults to 5 seconds
// - max open connections defaults to 1
// - WAL mode is enabled by default
type OpenOptions struct {
	// BusyTimeoutMillis controls the PRAGMA busy_timeout value.
	// Zero uses DefaultBusyTimeoutMillis.
	BusyTimeoutMillis int
	// MaxOpenConns controls the database/sql max open connections setting.
	// Zero uses DefaultMaxOpenConns.
	MaxOpenConns int
}

func DefaultOpenOptions() OpenOptions {
	return OpenOptions{
		BusyTimeoutMillis: DefaultBusyTimeoutMillis,
		MaxOpenConns:      DefaultMaxOpenConns,
	}
}

func Open(path string) (*sql.DB, error) {
	return OpenWithOptions(path, DefaultOpenOptions())
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
		maxOpenConns = DefaultMaxOpenConns
	}
	db.SetMaxOpenConns(maxOpenConns)
}

func applyPragmas(db *sql.DB, opts OpenOptions) error {
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("enable sqlite WAL mode: %w", err)
	}

	busyTimeoutMillis := opts.BusyTimeoutMillis
	if busyTimeoutMillis == 0 {
		busyTimeoutMillis = DefaultBusyTimeoutMillis
	}

	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMillis)); err != nil {
		return fmt.Errorf("set sqlite busy timeout: %w", err)
	}

	return nil
}
