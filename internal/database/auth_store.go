package database

import (
	"context"
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
	queries *db.Queries
}

var _ services.AuthStore = (*AuthStore)(nil)

func NewAuthStore(queries *db.Queries) *AuthStore {
	return &AuthStore{queries: queries}
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
