package database

import (
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
	if timeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", timeout)
	}
}
