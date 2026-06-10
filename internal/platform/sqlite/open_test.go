package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenCreatesSQLiteDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "app.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestOpenEnablesSQLiteForeignKeys(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, want 1", enabled)
	}
}

func TestOpenSetsSQLiteBusyTimeout(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout pragma: %v", err)
	}
	if timeout != DefaultBusyTimeoutMillis {
		t.Fatalf("busy_timeout = %d, want %d", timeout, DefaultBusyTimeoutMillis)
	}
}

func TestOpenWithOptionsOverridesDefaults(t *testing.T) {
	db, err := OpenWithOptions(filepath.Join(t.TempDir(), "app.db"), OpenOptions{
		BusyTimeoutMillis: 9000,
		MaxOpenConns:      2,
	})
	if err != nil {
		t.Fatalf("OpenWithOptions() error = %v", err)
	}
	defer db.Close()

	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout pragma: %v", err)
	}
	if timeout != 9000 {
		t.Fatalf("busy_timeout = %d, want 9000", timeout)
	}

	if maxOpen := db.Stats().MaxOpenConnections; maxOpen != 2 {
		t.Fatalf("MaxOpenConnections = %d, want 2", maxOpen)
	}
}

func TestOpenEnablesWALAndNormalSynchronous(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode pragma: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var synchronous int
	if err := db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("query synchronous pragma: %v", err)
	}
	if synchronous != 1 {
		t.Fatalf("synchronous = %d, want 1 (NORMAL)", synchronous)
	}
}

func TestOpenAppliesPragmasToEveryPooledConnection(t *testing.T) {
	db, err := OpenWithOptions(filepath.Join(t.TempDir(), "app.db"), OpenOptions{MaxOpenConns: 3})
	if err != nil {
		t.Fatalf("OpenWithOptions() error = %v", err)
	}
	defer db.Close()

	// Hold distinct connections open simultaneously so each check runs on a
	// different connection, not whichever one Exec'd a pragma at open time.
	ctx := context.Background()
	for i := range 3 {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn() error = %v", err)
		}
		defer conn.Close()

		var enabled int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			t.Fatalf("query foreign_keys pragma on conn %d: %v", i, err)
		}
		if enabled != 1 {
			t.Fatalf("foreign_keys on conn %d = %d, want 1", i, enabled)
		}
	}
}

func TestDefaultOpenOptions(t *testing.T) {
	opts := DefaultOpenOptions()

	if opts.BusyTimeoutMillis != DefaultBusyTimeoutMillis {
		t.Fatalf("BusyTimeoutMillis = %d, want %d", opts.BusyTimeoutMillis, DefaultBusyTimeoutMillis)
	}

	if opts.MaxOpenConns != DefaultMaxOpenConns {
		t.Fatalf("MaxOpenConns = %d, want %d", opts.MaxOpenConns, DefaultMaxOpenConns)
	}
}
