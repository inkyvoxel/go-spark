package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
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

	db, err := sql.Open("sqlite", dsn(path, opts))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	configureConnectionPool(db, opts)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	return db, nil
}

// dsn builds a connection string that applies the pragmas to every new
// connection. foreign_keys and busy_timeout are per-connection settings, so
// they must live in the DSN: applying them with Exec on the pool would only
// reach whichever single connection served that call, silently skipping the
// rest when MaxOpenConns is raised above one.
func dsn(path string, opts OpenOptions) string {
	busyTimeoutMillis := opts.BusyTimeoutMillis
	if busyTimeoutMillis == 0 {
		busyTimeoutMillis = DefaultBusyTimeoutMillis
	}

	params := url.Values{}
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMillis))
	params.Add("_pragma", "journal_mode(WAL)")
	// NORMAL is the recommended durability level with WAL: fsync on
	// checkpoint rather than every transaction.
	params.Add("_pragma", "synchronous(NORMAL)")

	return "file:" + path + "?" + params.Encode()
}

func configureConnectionPool(db *sql.DB, opts OpenOptions) {
	maxOpenConns := opts.MaxOpenConns
	if maxOpenConns == 0 {
		maxOpenConns = DefaultMaxOpenConns
	}
	db.SetMaxOpenConns(maxOpenConns)
}
