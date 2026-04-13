package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	db "go-starter/internal/db/generated"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidSession     = errors.New("invalid session")
)

type AuthService struct {
	queries         *db.Queries
	sessionDuration time.Duration
	bcryptCost      int
	tokenBytes      int
}

type AuthOptions struct {
	SessionDuration time.Duration
	BcryptCost      int
	TokenBytes      int
}

func NewAuthService(queries *db.Queries, opts AuthOptions) *AuthService {
	sessionDuration := opts.SessionDuration
	if sessionDuration == 0 {
		sessionDuration = 7 * 24 * time.Hour
	}

	bcryptCost := opts.BcryptCost
	if bcryptCost == 0 {
		bcryptCost = bcrypt.DefaultCost
	}

	tokenBytes := opts.TokenBytes
	if tokenBytes == 0 {
		tokenBytes = 32
	}

	return &AuthService{
		queries:         queries,
		sessionDuration: sessionDuration,
		bcryptCost:      bcryptCost,
		tokenBytes:      tokenBytes,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (db.User, error) {
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return db.User{}, ErrInvalidEmail
	}
	if password == "" {
		return db.User{}, ErrInvalidPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return db.User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: string(hash),
	})
	if err != nil {
		return db.User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (db.User, db.Session, error) {
	user, err := s.queries.GetUserByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, sql.ErrNoRows) {
		return db.User{}, db.Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return db.User{}, db.Session{}, fmt.Errorf("get user by email: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return db.User{}, db.Session{}, ErrInvalidCredentials
	}

	token, err := generateToken(s.tokenBytes)
	if err != nil {
		return db.User{}, db.Session{}, fmt.Errorf("generate session token: %w", err)
	}

	session, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(s.sessionDuration),
	})
	if err != nil {
		return db.User{}, db.Session{}, fmt.Errorf("create session: %w", err)
	}

	return user, session, nil
}

func (s *AuthService) UserBySessionToken(ctx context.Context, token string) (db.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return db.User{}, ErrInvalidSession
	}

	user, err := s.queries.GetUserBySessionToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return db.User{}, ErrInvalidSession
	}
	if err != nil {
		return db.User{}, fmt.Errorf("get user by session token: %w", err)
	}

	return user, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidSession
	}

	if err := s.queries.DeleteSessionByToken(ctx, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (s *AuthService) DeleteExpiredSessions(ctx context.Context) error {
	if err := s.queries.DeleteExpiredSessions(ctx); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}

	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func generateToken(bytes int) (string, error) {
	if bytes < 32 {
		bytes = 32
	}

	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	return hex.EncodeToString(buffer), nil
}
