package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"

	"golang.org/x/crypto/bcrypt"
)

func TestAuthServiceRegisterHashesPassword(t *testing.T) {
	service := newTestAuthService(t)

	user, err := service.Register(context.Background(), "  USER@example.COM  ", "correct horse battery staple")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if user.Email != "user@example.com" {
		t.Fatalf("Email = %q, want %q", user.Email, "user@example.com")
	}
	if user.PasswordHash == "correct horse battery staple" {
		t.Fatal("PasswordHash stores plaintext password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("correct horse battery staple")); err != nil {
		t.Fatalf("CompareHashAndPassword() error = %v", err)
	}
}

func TestAuthServiceRegisterValidatesInput(t *testing.T) {
	service := newTestAuthService(t)

	if _, err := service.Register(context.Background(), "not-an-email", "password"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("Register() error = %v, want %v", err, ErrInvalidEmail)
	}
	if _, err := service.Register(context.Background(), "test@example", "password"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("Register() error = %v, want %v", err, ErrInvalidEmail)
	}
	if _, err := service.Register(context.Background(), "user@example.com", ""); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("Register() error = %v, want %v", err, ErrInvalidPassword)
	}
	if _, err := service.Register(context.Background(), "user@example.com", "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("Register() error = %v, want %v", err, ErrInvalidPassword)
	}
}

func TestAuthServiceRegisterRejectsDuplicateEmail(t *testing.T) {
	service := newTestAuthService(t)

	if _, err := service.Register(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := service.Register(context.Background(), "USER@example.com", "password"); !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("Register() error = %v, want %v", err, ErrEmailAlreadyRegistered)
	}
}

func TestAuthServiceLoginCreatesSession(t *testing.T) {
	service := newTestAuthService(t)

	registered, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	user, session, err := service.Login(context.Background(), "USER@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if user.ID != registered.ID {
		t.Fatalf("logged in user ID = %d, want %d", user.ID, registered.ID)
	}
	if session.UserID != registered.ID {
		t.Fatalf("session user ID = %d, want %d", session.UserID, registered.ID)
	}
	if len(session.Token) != 64 {
		t.Fatalf("session token length = %d, want %d", len(session.Token), 64)
	}
	if time.Until(session.ExpiresAt) <= 0 {
		t.Fatalf("session ExpiresAt = %s, want future time", session.ExpiresAt)
	}
}

func TestAuthServiceLoginRejectsInvalidCredentials(t *testing.T) {
	service := newTestAuthService(t)

	if _, err := service.Register(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, _, err := service.Login(context.Background(), "user@example.com", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}

	_, _, err = service.Login(context.Background(), "missing@example.com", "password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestAuthServiceUserBySessionTokenAndLogout(t *testing.T) {
	service := newTestAuthService(t)

	registered, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, session, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	user, err := service.UserBySessionToken(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("UserBySessionToken() error = %v", err)
	}
	if user.ID != registered.ID {
		t.Fatalf("session user ID = %d, want %d", user.ID, registered.ID)
	}

	if err := service.Logout(context.Background(), session.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	_, err = service.UserBySessionToken(context.Background(), session.Token)
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("UserBySessionToken() error = %v, want %v", err, ErrInvalidSession)
	}
}

func newTestAuthService(t *testing.T) *AuthService {
	t.Helper()

	return NewAuthService(newFakeAuthStore(), AuthOptions{
		SessionDuration: time.Hour,
		BcryptCost:      bcrypt.MinCost,
		PasswordMinLen:  8,
	})
}

type fakeAuthStore struct {
	nextUserID    int64
	nextSessionID int64
	usersByEmail  map[string]db.User
	usersByID     map[int64]db.User
	sessions      map[string]db.Session
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		nextUserID:    1,
		nextSessionID: 1,
		usersByEmail:  make(map[string]db.User),
		usersByID:     make(map[int64]db.User),
		sessions:      make(map[string]db.Session),
	}
}

func (s *fakeAuthStore) CreateUser(ctx context.Context, email, passwordHash string) (db.User, error) {
	if _, ok := s.usersByEmail[email]; ok {
		return db.User{}, ErrEmailAlreadyRegistered
	}

	user := db.User{
		ID:           s.nextUserID,
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	}
	s.nextUserID++
	s.usersByEmail[email] = user
	s.usersByID[user.ID] = user

	return user, nil
}

func (s *fakeAuthStore) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	user, ok := s.usersByEmail[email]
	if !ok {
		return db.User{}, sql.ErrNoRows
	}

	return user, nil
}

func (s *fakeAuthStore) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) (db.Session, error) {
	session := db.Session{
		ID:        s.nextSessionID,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	s.nextSessionID++
	s.sessions[token] = session

	return session, nil
}

func (s *fakeAuthStore) GetUserBySessionToken(ctx context.Context, token string) (db.User, error) {
	session, ok := s.sessions[token]
	if !ok || !session.ExpiresAt.After(time.Now().UTC()) {
		return db.User{}, sql.ErrNoRows
	}

	user, ok := s.usersByID[session.UserID]
	if !ok {
		return db.User{}, sql.ErrNoRows
	}

	return user, nil
}

func (s *fakeAuthStore) DeleteSessionByToken(ctx context.Context, token string) error {
	delete(s.sessions, token)
	return nil
}
