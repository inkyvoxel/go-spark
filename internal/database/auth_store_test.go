package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
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
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_token_hash_idx ON sessions(token_hash);
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

CREATE TABLE password_reset_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX password_reset_tokens_user_id_idx ON password_reset_tokens(user_id);
CREATE INDEX password_reset_tokens_token_hash_idx ON password_reset_tokens(token_hash);

CREATE TABLE email_change_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    new_email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX email_change_tokens_user_id_idx ON email_change_tokens(user_id);
CREATE INDEX email_change_tokens_token_hash_idx ON email_change_tokens(token_hash);
CREATE INDEX email_change_tokens_new_email_idx ON email_change_tokens(new_email);

CREATE TABLE email_outbox (
    id INTEGER PRIMARY KEY,
    sender TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    text_body TEXT NOT NULL,
    html_body TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    available_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    claimed_at TIMESTAMP,
    claim_expires_at TIMESTAMP,
    claim_token TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX email_outbox_pending_idx ON email_outbox(status, available_at);
CREATE INDEX email_outbox_claim_expiry_idx ON email_outbox(status, claim_expires_at);
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
	if session.TokenHash != "token" {
		t.Fatalf("CreateSession() TokenHash = %q, want %q", session.TokenHash, "token")
	}
	var storedTokenHash string
	if err := store.db.QueryRowContext(context.Background(), "SELECT token_hash FROM sessions WHERE id = ?", session.ID).Scan(&storedTokenHash); err != nil {
		t.Fatalf("select session token_hash: %v", err)
	}
	if storedTokenHash != "token" {
		t.Fatalf("stored token_hash = %q, want %q", storedTokenHash, "token")
	}

	bySession, err := store.GetUserBySessionTokenHash(context.Background(), "token")
	if err != nil {
		t.Fatalf("GetUserBySessionTokenHash() error = %v", err)
	}
	if bySession.ID != user.ID {
		t.Fatalf("GetUserBySessionTokenHash() ID = %d, want %d", bySession.ID, user.ID)
	}

	if err := store.DeleteSessionByTokenHash(context.Background(), "token"); err != nil {
		t.Fatalf("DeleteSessionByTokenHash() error = %v", err)
	}

	_, err = store.GetUserBySessionTokenHash(context.Background(), "token")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetUserBySessionTokenHash() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStoreUpdateUserPasswordHash(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "old-hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := store.UpdateUserPasswordHash(context.Background(), user.ID, "new-hash"); err != nil {
		t.Fatalf("UpdateUserPasswordHash() error = %v", err)
	}

	updated, err := store.GetUserByEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if updated.PasswordHash != "new-hash" {
		t.Fatalf("PasswordHash = %q, want %q", updated.PasswordHash, "new-hash")
	}
}

func TestAuthStoreDeleteSessionsByUserID(t *testing.T) {
	store := newTestAuthStore(t)

	userOne, err := store.CreateUser(context.Background(), "user1@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	userTwo, err := store.CreateUser(context.Background(), "user2@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	userOneSessionA, err := store.CreateSession(context.Background(), userOne.ID, "token-1a", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	userOneSessionB, err := store.CreateSession(context.Background(), userOne.ID, "token-1b", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	userTwoSession, err := store.CreateSession(context.Background(), userTwo.ID, "token-2", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := store.DeleteSessionsByUserID(context.Background(), userOne.ID); err != nil {
		t.Fatalf("DeleteSessionsByUserID() error = %v", err)
	}

	_, err = store.GetUserBySessionTokenHash(context.Background(), userOneSessionA.TokenHash)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetUserBySessionTokenHash() token-1a error = %v, want %v", err, sql.ErrNoRows)
	}
	_, err = store.GetUserBySessionTokenHash(context.Background(), userOneSessionB.TokenHash)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetUserBySessionTokenHash() token-1b error = %v, want %v", err, sql.ErrNoRows)
	}

	remaining, err := store.GetUserBySessionTokenHash(context.Background(), userTwoSession.TokenHash)
	if err != nil {
		t.Fatalf("GetUserBySessionTokenHash() token-2 error = %v", err)
	}
	if remaining.ID != userTwo.ID {
		t.Fatalf("remaining session user ID = %d, want %d", remaining.ID, userTwo.ID)
	}
}

func TestAuthStoreListActiveSessionsByUserID(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	otherUser, err := store.CreateUser(context.Background(), "other@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	now := time.Now().UTC()
	activeA, err := store.CreateSession(context.Background(), user.ID, "token-a", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	activeB, err := store.CreateSession(context.Background(), user.ID, "token-b", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := store.CreateSession(context.Background(), user.ID, "token-expired", now.Add(-time.Minute)); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := store.CreateSession(context.Background(), otherUser.ID, "token-other-user", now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	sessions, err := store.ListActiveSessionsByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListActiveSessionsByUserID() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("active session count = %d, want 2", len(sessions))
	}

	found := map[int64]bool{
		activeA.ID: false,
		activeB.ID: false,
	}
	for _, session := range sessions {
		if _, ok := found[session.ID]; ok {
			found[session.ID] = true
		}
	}
	for id, ok := range found {
		if !ok {
			t.Fatalf("session ID %d missing from active sessions", id)
		}
	}
}

func TestAuthStoreDeleteOtherSessionsByUserIDAndTokenHash(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	otherUser, err := store.CreateUser(context.Background(), "other@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	now := time.Now().UTC()
	current, err := store.CreateSession(context.Background(), user.ID, "token-current", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	otherA, err := store.CreateSession(context.Background(), user.ID, "token-other-a", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	otherB, err := store.CreateSession(context.Background(), user.ID, "token-other-b", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	otherUserSession, err := store.CreateSession(context.Background(), otherUser.ID, "token-different-user", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	deleted, err := store.DeleteOtherSessionsByUserIDAndTokenHash(context.Background(), user.ID, current.TokenHash)
	if err != nil {
		t.Fatalf("DeleteOtherSessionsByUserIDAndTokenHash() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted sessions = %d, want 2", deleted)
	}

	if _, err := store.GetUserBySessionTokenHash(context.Background(), current.TokenHash); err != nil {
		t.Fatalf("current session lookup error = %v", err)
	}
	if _, err := store.GetUserBySessionTokenHash(context.Background(), otherA.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("other session A lookup error = %v, want %v", err, sql.ErrNoRows)
	}
	if _, err := store.GetUserBySessionTokenHash(context.Background(), otherB.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("other session B lookup error = %v, want %v", err, sql.ErrNoRows)
	}
	remaining, err := store.GetUserBySessionTokenHash(context.Background(), otherUserSession.TokenHash)
	if err != nil {
		t.Fatalf("other user session lookup error = %v", err)
	}
	if remaining.ID != otherUser.ID {
		t.Fatalf("other user session user ID = %d, want %d", remaining.ID, otherUser.ID)
	}
}

func TestAuthStoreDeleteSessionByIDAndUserIDAndTokenHashNot(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	otherUser, err := store.CreateUser(context.Background(), "other@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	now := time.Now().UTC()
	current, err := store.CreateSession(context.Background(), user.ID, "token-current", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	other, err := store.CreateSession(context.Background(), user.ID, "token-other", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	otherUserSession, err := store.CreateSession(context.Background(), otherUser.ID, "token-other-user", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	deleted, err := store.DeleteSessionByIDAndUserIDAndTokenHashNot(context.Background(), other.ID, user.ID, current.TokenHash)
	if err != nil {
		t.Fatalf("DeleteSessionByIDAndUserIDAndTokenHashNot() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted rows = %d, want 1", deleted)
	}
	if _, err := store.GetUserBySessionTokenHash(context.Background(), other.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted session lookup error = %v, want %v", err, sql.ErrNoRows)
	}

	deleted, err = store.DeleteSessionByIDAndUserIDAndTokenHashNot(context.Background(), current.ID, user.ID, current.TokenHash)
	if err != nil {
		t.Fatalf("DeleteSessionByIDAndUserIDAndTokenHashNot() current error = %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted rows for current session = %d, want 0", deleted)
	}

	deleted, err = store.DeleteSessionByIDAndUserIDAndTokenHashNot(context.Background(), otherUserSession.ID, user.ID, current.TokenHash)
	if err != nil {
		t.Fatalf("DeleteSessionByIDAndUserIDAndTokenHashNot() cross-user error = %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted rows for cross-user session = %d, want 0", deleted)
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

func TestAuthStoreCreateUserWithEmailVerification(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUserWithEmailVerification(
		context.Background(),
		services.CreateUserWithEmailVerificationParams{
			Email:          "user@example.com",
			PasswordHash:   "hash",
			TokenHash:      "token-hash",
			TokenExpiresAt: now.Add(time.Hour),
			ConfirmationEmail: email.Message{
				From:     "sender@example.com",
				To:       "user@example.com",
				Subject:  "Confirm your email address",
				TextBody: "Confirm using this link.",
				HTMLBody: "<p>Confirm</p>",
			},
			EmailAvailableAt: now,
		},
	)
	if err != nil {
		t.Fatalf("CreateUserWithEmailVerification() error = %v", err)
	}

	token, err := store.queries.ConsumeEmailVerificationToken(context.Background(), db.ConsumeEmailVerificationTokenParams{
		ConsumedAt: sql.NullTime{Time: now, Valid: true},
		TokenHash:  "token-hash",
		ExpiresAt:  now,
	})
	if err != nil {
		t.Fatalf("ConsumeEmailVerificationToken() error = %v", err)
	}
	if token.UserID != user.ID {
		t.Fatalf("verification token user ID = %d, want %d", token.UserID, user.ID)
	}

	claimed, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now.Add(time.Second), Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(3 * time.Minute), Valid: true},
		ClaimToken:     "test-claim",
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed email count = %d, want 1", len(claimed))
	}
	if claimed[0].Recipient != "user@example.com" {
		t.Fatalf("claimed recipient = %q, want user@example.com", claimed[0].Recipient)
	}
}

func TestAuthStoreCreateUserWithEmailVerificationRollsBackOnOutboxError(t *testing.T) {
	conn := newAuthStoreTestDB(t)
	store := NewAuthStore(conn)
	if _, err := conn.Exec("DROP TABLE email_outbox"); err != nil {
		t.Fatalf("drop email_outbox: %v", err)
	}
	now := time.Now().UTC()

	_, err := store.CreateUserWithEmailVerification(
		context.Background(),
		services.CreateUserWithEmailVerificationParams{
			Email:             "user@example.com",
			PasswordHash:      "hash",
			TokenHash:         "token-hash",
			TokenExpiresAt:    now.Add(time.Hour),
			ConfirmationEmail: email.Message{},
			EmailAvailableAt:  now,
		},
	)
	if err == nil {
		t.Fatal("CreateUserWithEmailVerification() error = nil, want error")
	}

	_, err = store.GetUserByEmail(context.Background(), "user@example.com")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetUserByEmail() error = %v, want %v", err, sql.ErrNoRows)
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

func TestAuthStorePasswordResetTokenFlow(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	token, err := store.CreatePasswordResetToken(context.Background(), user.ID, "token-hash", expiresAt)
	if err != nil {
		t.Fatalf("CreatePasswordResetToken() error = %v", err)
	}
	if token.UserID != user.ID {
		t.Fatalf("password reset token user ID = %d, want %d", token.UserID, user.ID)
	}

	valid, err := store.GetValidPasswordResetTokenByHash(context.Background(), "token-hash", time.Now().UTC())
	if err != nil {
		t.Fatalf("GetValidPasswordResetTokenByHash() error = %v", err)
	}
	if valid.UserID != user.ID {
		t.Fatalf("valid token user ID = %d, want %d", valid.UserID, user.ID)
	}

	consumed, err := store.ConsumePasswordResetToken(context.Background(), "token-hash", time.Now().UTC())
	if err != nil {
		t.Fatalf("ConsumePasswordResetToken() error = %v", err)
	}
	if !consumed.ConsumedAt.Valid {
		t.Fatal("ConsumedAt.Valid = false, want true")
	}

	_, err = store.GetValidPasswordResetTokenByHash(context.Background(), "token-hash", time.Now().UTC())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetValidPasswordResetTokenByHash() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStorePasswordResetTokenRejectsExpiredToken(t *testing.T) {
	store := newTestAuthStore(t)

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := store.CreatePasswordResetToken(context.Background(), user.ID, "token-hash", time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("CreatePasswordResetToken() error = %v", err)
	}

	_, err = store.GetValidPasswordResetTokenByHash(context.Background(), "token-hash", time.Now().UTC())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetValidPasswordResetTokenByHash() error = %v, want %v", err, sql.ErrNoRows)
	}
	_, err = store.ConsumePasswordResetToken(context.Background(), "token-hash", time.Now().UTC())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ConsumePasswordResetToken() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStoreRequestPasswordReset(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	err = store.RequestPasswordReset(context.Background(), services.RequestPasswordResetParams{
		UserID:         user.ID,
		TokenHash:      "reset-token-hash",
		TokenExpiresAt: now.Add(time.Hour),
		PasswordResetEmail: email.Message{
			From:     "sender@example.com",
			To:       "user@example.com",
			Subject:  "Reset your password",
			TextBody: "Reset using this link.",
			HTMLBody: "<p>Reset</p>",
		},
		EmailAvailableAt: now,
	})
	if err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}

	token, err := store.queries.ConsumePasswordResetToken(context.Background(), db.ConsumePasswordResetTokenParams{
		ConsumedAt: sql.NullTime{Time: now, Valid: true},
		TokenHash:  "reset-token-hash",
		ExpiresAt:  now,
	})
	if err != nil {
		t.Fatalf("ConsumePasswordResetToken() error = %v", err)
	}
	if token.UserID != user.ID {
		t.Fatalf("password reset token user ID = %d, want %d", token.UserID, user.ID)
	}

	claimed, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now.Add(time.Second), Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(3 * time.Minute), Valid: true},
		ClaimToken:     "test-claim",
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed email count = %d, want 1", len(claimed))
	}
	if claimed[0].Recipient != "user@example.com" {
		t.Fatalf("claimed recipient = %q, want user@example.com", claimed[0].Recipient)
	}
}

func TestAuthStoreResendEmailVerification(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	err = store.ResendEmailVerification(context.Background(), services.ResendEmailVerificationParams{
		UserID:         user.ID,
		TokenHash:      "resend-token-hash",
		TokenExpiresAt: now.Add(time.Hour),
		ConfirmationEmail: email.Message{
			From:     "sender@example.com",
			To:       "user@example.com",
			Subject:  "Confirm your email address",
			TextBody: "Confirm using this link.",
			HTMLBody: "<p>Confirm</p>",
		},
		EmailAvailableAt: now,
	})
	if err != nil {
		t.Fatalf("ResendEmailVerification() error = %v", err)
	}

	token, err := store.queries.ConsumeEmailVerificationToken(context.Background(), db.ConsumeEmailVerificationTokenParams{
		ConsumedAt: sql.NullTime{Time: now, Valid: true},
		TokenHash:  "resend-token-hash",
		ExpiresAt:  now,
	})
	if err != nil {
		t.Fatalf("ConsumeEmailVerificationToken() error = %v", err)
	}
	if token.UserID != user.ID {
		t.Fatalf("verification token user ID = %d, want %d", token.UserID, user.ID)
	}

	claimed, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now.Add(time.Second), Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(3 * time.Minute), Valid: true},
		ClaimToken:     "test-claim",
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed email count = %d, want 1", len(claimed))
	}
}

func TestAuthStoreResendEmailVerificationRollsBackOnOutboxError(t *testing.T) {
	conn := newAuthStoreTestDB(t)
	store := NewAuthStore(conn)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := conn.Exec("DROP TABLE email_outbox"); err != nil {
		t.Fatalf("drop email_outbox: %v", err)
	}

	err = store.ResendEmailVerification(context.Background(), services.ResendEmailVerificationParams{
		UserID:         user.ID,
		TokenHash:      "resend-token-hash",
		TokenExpiresAt: now.Add(time.Hour),
		ConfirmationEmail: email.Message{
			From:     "sender@example.com",
			To:       "user@example.com",
			Subject:  "Confirm your email address",
			TextBody: "Confirm using this link.",
		},
		EmailAvailableAt: now,
	})
	if err == nil {
		t.Fatal("ResendEmailVerification() error = nil, want error")
	}

	_, err = store.queries.ConsumeEmailVerificationToken(context.Background(), db.ConsumeEmailVerificationTokenParams{
		ConsumedAt: sql.NullTime{Time: now, Valid: true},
		TokenHash:  "resend-token-hash",
		ExpiresAt:  now,
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ConsumeEmailVerificationToken() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStoreRequestEmailChange(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	err = store.RequestEmailChange(context.Background(), services.RequestEmailChangeParams{
		UserID:         user.ID,
		NewEmail:       "new@example.com",
		TokenHash:      "email-change-token-hash",
		TokenExpiresAt: now.Add(time.Hour),
		EmailChangeVerifyEmail: email.Message{
			From:     "sender@example.com",
			To:       "new@example.com",
			Subject:  "Verify your new email address",
			TextBody: "Verify using this link.",
			HTMLBody: "<p>Verify using this link.</p>",
		},
		EmailAvailableAt: now,
	})
	if err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}

	var tokenCount int
	if err := store.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM email_change_tokens WHERE user_id = ? AND new_email = ? AND token_hash = ?", user.ID, "new@example.com", "email-change-token-hash").Scan(&tokenCount); err != nil {
		t.Fatalf("count email change tokens: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("email change token count = %d, want 1", tokenCount)
	}

	claimed, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now.Add(time.Second), Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(3 * time.Minute), Valid: true},
		ClaimToken:     "test-claim",
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed email count = %d, want 1", len(claimed))
	}
	if claimed[0].Recipient != "new@example.com" {
		t.Fatalf("claimed recipient = %q, want new email", claimed[0].Recipient)
	}
}

func TestAuthStoreChangeEmailImmediately(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	session, err := store.CreateSession(context.Background(), user.ID, "session-token", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	changed, err := store.ChangeEmailImmediately(context.Background(), services.ChangeEmailImmediatelyParams{
		UserID:                 user.ID,
		NewEmail:               "new@example.com",
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: "sender@example.com"},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     true,
	})
	if err != nil {
		t.Fatalf("ChangeEmailImmediately() error = %v", err)
	}
	if changed.Email != "new@example.com" {
		t.Fatalf("changed email = %q, want %q", changed.Email, "new@example.com")
	}
	if !changed.EmailVerifiedAt.Valid {
		t.Fatal("EmailVerifiedAt.Valid = false, want true")
	}

	_, err = store.GetUserByEmail(context.Background(), "user@example.com")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old email lookup error = %v, want %v", err, sql.ErrNoRows)
	}
	if _, err := store.GetUserByEmail(context.Background(), "new@example.com"); err != nil {
		t.Fatalf("new email lookup error = %v", err)
	}
	if _, err := store.GetUserBySessionTokenHash(context.Background(), session.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old session lookup error = %v, want %v", err, sql.ErrNoRows)
	}

	claimed, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now.Add(time.Second), Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(3 * time.Minute), Valid: true},
		ClaimToken:     "test-claim",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}
	var noticeFound bool
	for _, item := range claimed {
		if item.Recipient == "<user@example.com>" && item.Subject == "Your email address was changed" {
			noticeFound = true
		}
	}
	if !noticeFound {
		t.Fatalf("claimed emails = %#v, want old email notice", claimed)
	}
}

func TestAuthStoreChangeEmailImmediatelySkipsOldEmailNoticeWhenDisabled(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	_, err = store.ChangeEmailImmediately(context.Background(), services.ChangeEmailImmediatelyParams{
		UserID:                 user.ID,
		NewEmail:               "new@example.com",
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: "sender@example.com"},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     false,
	})
	if err != nil {
		t.Fatalf("ChangeEmailImmediately() error = %v", err)
	}

	claimed, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now.Add(time.Second), Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(3 * time.Minute), Valid: true},
		ClaimToken:     "test-claim",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed emails = %#v, want none", claimed)
	}
}

func TestAuthStoreConfirmEmailChange(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	session, err := store.CreateSession(context.Background(), user.ID, "session-token", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := store.RequestEmailChange(context.Background(), services.RequestEmailChangeParams{
		UserID:         user.ID,
		NewEmail:       "new@example.com",
		TokenHash:      "email-change-token-hash",
		TokenExpiresAt: now.Add(time.Hour),
		EmailChangeVerifyEmail: email.Message{
			From:     "sender@example.com",
			To:       "new@example.com",
			Subject:  "Verify your new email address",
			TextBody: "Verify using this link.",
		},
		EmailAvailableAt: now,
	}); err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}

	changed, err := store.ConfirmEmailChange(context.Background(), services.ConfirmEmailChangeParams{
		TokenHash:              "email-change-token-hash",
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: "sender@example.com"},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     true,
	})
	if err != nil {
		t.Fatalf("ConfirmEmailChange() error = %v", err)
	}
	if changed.Email != "new@example.com" {
		t.Fatalf("changed email = %q, want new email", changed.Email)
	}
	if !changed.EmailVerifiedAt.Valid {
		t.Fatal("EmailVerifiedAt.Valid = false, want true")
	}

	_, err = store.GetUserByEmail(context.Background(), "user@example.com")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old email lookup error = %v, want %v", err, sql.ErrNoRows)
	}
	if _, err := store.GetUserByEmail(context.Background(), "new@example.com"); err != nil {
		t.Fatalf("new email lookup error = %v", err)
	}
	if _, err := store.GetUserBySessionTokenHash(context.Background(), session.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old session lookup error = %v, want %v", err, sql.ErrNoRows)
	}

	claimed, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now.Add(time.Second), Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(3 * time.Minute), Valid: true},
		ClaimToken:     "test-claim",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}
	var noticeFound bool
	for _, item := range claimed {
		if item.Recipient == "<user@example.com>" && item.Subject == "Your email address was changed" {
			noticeFound = true
		}
	}
	if !noticeFound {
		t.Fatalf("claimed emails = %#v, want old email notice", claimed)
	}

	_, err = store.ConfirmEmailChange(context.Background(), services.ConfirmEmailChangeParams{
		TokenHash:              "email-change-token-hash",
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: "sender@example.com"},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     true,
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ConfirmEmailChange() consumed token error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStoreConfirmEmailChangeRejectsExpiredToken(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err := store.RequestEmailChange(context.Background(), services.RequestEmailChangeParams{
		UserID:         user.ID,
		NewEmail:       "new@example.com",
		TokenHash:      "expired-token-hash",
		TokenExpiresAt: now.Add(-time.Minute),
		EmailChangeVerifyEmail: email.Message{
			From:     "sender@example.com",
			To:       "new@example.com",
			Subject:  "Verify your new email address",
			TextBody: "Verify using this link.",
		},
		EmailAvailableAt: now,
	}); err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}

	_, err = store.ConfirmEmailChange(context.Background(), services.ConfirmEmailChangeParams{
		TokenHash:              "expired-token-hash",
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: "sender@example.com"},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     true,
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ConfirmEmailChange() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestAuthStoreConfirmEmailChangeRejectsAlreadyOwnedEmail(t *testing.T) {
	store := newTestAuthStore(t)
	now := time.Now().UTC()

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err := store.RequestEmailChange(context.Background(), services.RequestEmailChangeParams{
		UserID:         user.ID,
		NewEmail:       "new@example.com",
		TokenHash:      "email-change-token-hash",
		TokenExpiresAt: now.Add(time.Hour),
		EmailChangeVerifyEmail: email.Message{
			From:     "sender@example.com",
			To:       "new@example.com",
			Subject:  "Verify your new email address",
			TextBody: "Verify using this link.",
		},
		EmailAvailableAt: now,
	}); err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}
	if _, err := store.CreateUser(context.Background(), "new@example.com", "hash"); err != nil {
		t.Fatalf("CreateUser() competing email error = %v", err)
	}

	_, err = store.ConfirmEmailChange(context.Background(), services.ConfirmEmailChangeParams{
		TokenHash:              "email-change-token-hash",
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: "sender@example.com"},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     true,
	})
	if !errors.Is(err, services.ErrEmailAlreadyRegistered) {
		t.Fatalf("ConfirmEmailChange() error = %v, want %v", err, services.ErrEmailAlreadyRegistered)
	}

	found, err := store.GetUserByEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail() original error = %v", err)
	}
	if found.ID != user.ID {
		t.Fatalf("original email user ID = %d, want %d", found.ID, user.ID)
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
