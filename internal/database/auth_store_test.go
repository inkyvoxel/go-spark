package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/inkyvoxel/go-spark/internal/services"
)

const authStoreTestSchema = `
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    email_verified_at TIMESTAMP
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

CREATE TABLE email_verification_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX email_verification_tokens_user_id_idx ON email_verification_tokens(user_id);
CREATE INDEX email_verification_tokens_token_hash_idx ON email_verification_tokens(token_hash);
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
	store := NewAuthStore(conn)

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

func TestAuthStoreEmailVerificationFlow(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	token, err := store.CreateEmailVerificationToken(context.Background(), user.ID, "token-hash", expiresAt)
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken() error = %v", err)
	}
	if token.UserID != user.ID {
		t.Fatalf("verification token user ID = %d, want %d", token.UserID, user.ID)
	}
	if token.TokenHash != "token-hash" {
		t.Fatalf("verification token hash = %q, want %q", token.TokenHash, "token-hash")
	}

	verifiedAt := time.Now().UTC()
	verified, err := store.VerifyEmailByTokenHash(context.Background(), "token-hash", verifiedAt)
	if err != nil {
		t.Fatalf("VerifyEmailByTokenHash() error = %v", err)
	}
	if verified.ID != user.ID {
		t.Fatalf("verified user ID = %d, want %d", verified.ID, user.ID)
	}
	if !verified.EmailVerifiedAt.Valid {
		t.Fatal("EmailVerifiedAt.Valid = false, want true")
	}

	_, err = store.VerifyEmailByTokenHash(context.Background(), "token-hash", time.Now().UTC())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("VerifyEmailByTokenHash() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStoreEmailVerificationRejectsExpiredToken(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := store.CreateEmailVerificationToken(context.Background(), user.ID, "token-hash", time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("CreateEmailVerificationToken() error = %v", err)
	}

	_, err = store.VerifyEmailByTokenHash(context.Background(), "token-hash", time.Now().UTC())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("VerifyEmailByTokenHash() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func newTestAuthStore(t *testing.T) *AuthStore {
	t.Helper()

	return NewAuthStore(newAuthStoreTestDB(t))
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
