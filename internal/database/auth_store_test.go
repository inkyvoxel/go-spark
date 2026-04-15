package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const authStoreTestSchema = `
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_token_idx ON sessions(token);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);
`

func TestAuthStoreCreateUserTranslatesDuplicateEmail(t *testing.T) {
	store := newTestAuthStore(t)

	if _, err := store.CreateUser(context.Background(), "user@example.com", "hash"); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	_, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if !errors.Is(err, services.ErrEmailAlreadyRegistered) {
		t.Fatalf("CreateUser() error = %v, want %v", err, services.ErrEmailAlreadyRegistered)
	}
}

func TestAuthStoreUserAndSessionFlow(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	found, err := store.GetUserByEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if found.ID != user.ID {
		t.Fatalf("GetUserByEmail() ID = %d, want %d", found.ID, user.ID)
	}

	session, err := store.CreateSession(context.Background(), user.ID, "token", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.UserID != user.ID {
		t.Fatalf("CreateSession() UserID = %d, want %d", session.UserID, user.ID)
	}

	bySession, err := store.GetUserBySessionToken(context.Background(), "token")
	if err != nil {
		t.Fatalf("GetUserBySessionToken() error = %v", err)
	}
	if bySession.ID != user.ID {
		t.Fatalf("GetUserBySessionToken() ID = %d, want %d", bySession.ID, user.ID)
	}

	if err := store.DeleteSessionByToken(context.Background(), "token"); err != nil {
		t.Fatalf("DeleteSessionByToken() error = %v", err)
	}

	_, err = store.GetUserBySessionToken(context.Background(), "token")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetUserBySessionToken() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStoreUnexpectedCreateUserErrorIsWrapped(t *testing.T) {
	conn := newAuthStoreTestDB(t)
	store := NewAuthStore(db.New(conn))

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err == nil {
		t.Fatal("CreateUser() error = nil, want error")
	}
	if errors.Is(err, services.ErrEmailAlreadyRegistered) {
		t.Fatalf("CreateUser() error = %v, did not want duplicate email error", err)
	}
	if !strings.Contains(err.Error(), "create user") {
		t.Fatalf("CreateUser() error = %v, want operation context", err)
	}
}

func newTestAuthStore(t *testing.T) *AuthStore {
	t.Helper()

	return NewAuthStore(db.New(newAuthStoreTestDB(t)))
}

func newAuthStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	if _, err := conn.Exec(authStoreTestSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return conn
}
