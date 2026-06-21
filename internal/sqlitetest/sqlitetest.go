// Package sqlitetest provides a SQLite database for tests, opened with the same
// connection tuning as production and migrated with the real goose migrations
// (no hand-copied schema, so tests cannot drift from the live schema).
package sqlitetest

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"

	appassets "github.com/inkyvoxel/go-spark"
	"github.com/inkyvoxel/go-spark/internal/platform/sqlite"
)

// goose keeps dialect and base FS in process-global state; set them once.
var gooseOnce sync.Once

// New returns a migrated, isolated SQLite database backed by a temp file. The
// connection and the file are cleaned up when the test finishes.
func New(t testing.TB) *sql.DB {
	t.Helper()

	gooseOnce.Do(func() {
		goose.SetBaseFS(appassets.FS)
		// "sqlite3" is a dialect goose always knows, so this never fails; panic
		// rather than t.Fatalf since Do runs under whichever test wins the race.
		if err := goose.SetDialect("sqlite3"); err != nil {
			panic("sqlitetest: set goose dialect: " + err.Error())
		}
	})

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := goose.UpContext(context.Background(), db, "migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}
