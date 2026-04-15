package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"

	"golang.org/x/crypto/bcrypt"
)

const DefaultPasswordMinLength = 12

var (
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrInvalidEmail           = errors.New("invalid email")
	ErrInvalidPassword        = errors.New("invalid password")
	ErrInvalidSession         = errors.New("invalid session")
)

type AuthService struct {
	store           AuthStore
	sessionDuration time.Duration
	bcryptCost      int
	tokenBytes      int
	passwordMinLen  int
}

type AuthStore interface {
	CreateUser(ctx context.Context, email, passwordHash string) (db.User, error)
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
	CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) (db.Session, error)
	GetUserBySessionToken(ctx context.Context, token string) (db.User, error)
	DeleteSessionByToken(ctx context.Context, token string) error
	DeleteExpiredSessions(ctx context.Context) error
}

type AuthOptions struct {
	SessionDuration time.Duration
	BcryptCost      int
	TokenBytes      int
	PasswordMinLen  int
}

func NewAuthService(store AuthStore, opts AuthOptions) *AuthService {
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

	passwordMinLen := opts.PasswordMinLen
	if passwordMinLen == 0 {
		passwordMinLen = DefaultPasswordMinLength
	}

	return &AuthService{
		store:           store,
		sessionDuration: sessionDuration,
		bcryptCost:      bcryptCost,
		tokenBytes:      tokenBytes,
		passwordMinLen:  passwordMinLen,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (db.User, error) {
	email = normalizeEmail(email)
	if !isValidEmail(email) {
		return db.User{}, ErrInvalidEmail
	}
	if utf8.RuneCountInString(password) < s.passwordMinLen {
		return db.User{}, ErrInvalidPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return db.User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, email, string(hash))
	if err != nil {
		if errors.Is(err, ErrEmailAlreadyRegistered) {
			return db.User{}, ErrEmailAlreadyRegistered
		}
		return db.User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (db.User, db.Session, error) {
	user, err := s.store.GetUserByEmail(ctx, normalizeEmail(email))
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

	session, err := s.store.CreateSession(ctx, user.ID, token, time.Now().UTC().Add(s.sessionDuration))
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

	user, err := s.store.GetUserBySessionToken(ctx, token)
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

	if err := s.store.DeleteSessionByToken(ctx, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (s *AuthService) DeleteExpiredSessions(ctx context.Context) error {
	if err := s.store.DeleteExpiredSessions(ctx); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}

	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isValidEmail(email string) bool {
	if len(email) > 254 {
		return false
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return false
	}

	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" || len(local) > 64 {
		return false
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}

	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}

	tld := labels[len(labels)-1]
	if len(tld) < 2 {
		return false
	}
	for _, r := range tld {
		if r >= 'a' && r <= 'z' {
			return true
		}
	}
	return false
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
