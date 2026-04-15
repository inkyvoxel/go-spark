package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type AuthStore struct {
	db      *sql.DB
	queries *db.Queries
}

var _ services.AuthStore = (*AuthStore)(nil)

func NewAuthStore(conn *sql.DB) *AuthStore {
	return &AuthStore{
		db:      conn,
		queries: db.New(conn),
	}
}

func (s *AuthStore) CreateUser(ctx context.Context, email, passwordHash string) (db.User, error) {
	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return db.User{}, services.ErrEmailAlreadyRegistered
		}
		return db.User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (s *AuthStore) CreateUserWithEmailVerification(ctx context.Context, params services.CreateUserWithEmailVerificationParams) (db.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.User{}, fmt.Errorf("begin register user transaction: %w", err)
	}
	defer tx.Rollback()

	queries := s.queries.WithTx(tx)
	user, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
	})
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return db.User{}, services.ErrEmailAlreadyRegistered
		}
		return db.User{}, fmt.Errorf("create user: %w", err)
	}

	if _, err := queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
		UserID:    user.ID,
		TokenHash: params.TokenHash,
		ExpiresAt: params.TokenExpiresAt,
	}); err != nil {
		return db.User{}, fmt.Errorf("create email verification token: %w", err)
	}

	if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
		Sender:      params.ConfirmationEmail.From,
		Recipient:   params.ConfirmationEmail.To,
		Subject:     params.ConfirmationEmail.Subject,
		TextBody:    params.ConfirmationEmail.TextBody,
		HtmlBody:    params.ConfirmationEmail.HTMLBody,
		AvailableAt: params.EmailAvailableAt,
	}); err != nil {
		return db.User{}, fmt.Errorf("enqueue confirmation email: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return db.User{}, fmt.Errorf("commit register user transaction: %w", err)
	}

	return user, nil
}

func (s *AuthStore) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return db.User{}, fmt.Errorf("get user by email: %w", err)
	}

	return user, nil
}

func (s *AuthStore) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) (db.Session, error) {
	session, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return db.Session{}, fmt.Errorf("create session: %w", err)
	}

	return session, nil
}

func (s *AuthStore) GetUserBySessionToken(ctx context.Context, token string) (db.User, error) {
	user, err := s.queries.GetUserBySessionToken(ctx, token)
	if err != nil {
		return db.User{}, fmt.Errorf("get user by session token: %w", err)
	}

	return user, nil
}

func (s *AuthStore) DeleteSessionByToken(ctx context.Context, token string) error {
	if err := s.queries.DeleteSessionByToken(ctx, token); err != nil {
		return fmt.Errorf("delete session by token: %w", err)
	}

	return nil
}

func (s *AuthStore) CreateEmailVerificationToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (db.EmailVerificationToken, error) {
	token, err := s.queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return db.EmailVerificationToken{}, fmt.Errorf("create email verification token: %w", err)
	}

	return token, nil
}

func (s *AuthStore) VerifyEmailByTokenHash(ctx context.Context, tokenHash string, verifiedAt time.Time) (db.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.User{}, fmt.Errorf("begin verify email transaction: %w", err)
	}
	defer tx.Rollback()

	queries := s.queries.WithTx(tx)
	token, err := queries.ConsumeEmailVerificationToken(ctx, db.ConsumeEmailVerificationTokenParams{
		ConsumedAt: sql.NullTime{Time: verifiedAt, Valid: true},
		TokenHash:  tokenHash,
		ExpiresAt:  verifiedAt,
	})
	if err != nil {
		return db.User{}, fmt.Errorf("consume email verification token: %w", err)
	}

	user, err := queries.MarkUserEmailVerified(ctx, db.MarkUserEmailVerifiedParams{
		EmailVerifiedAt: sql.NullTime{Time: verifiedAt, Valid: true},
		ID:              token.UserID,
	})
	if err != nil {
		return db.User{}, fmt.Errorf("mark user email verified: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return db.User{}, fmt.Errorf("commit verify email transaction: %w", err)
	}

	return user, nil
}

func isSQLiteUniqueConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	if sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return true
	}

	return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT && strings.Contains(strings.ToLower(sqliteErr.Error()), "unique")
}
