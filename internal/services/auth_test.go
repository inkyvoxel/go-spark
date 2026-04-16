package services

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
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
	matches, err := service.passwordHasher.Verify(user.PasswordHash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !matches {
		t.Fatal("Verify() = false, want true")
	}

	store := service.store.(*fakeAuthStore)
	if len(store.verificationTokens) != 1 {
		t.Fatalf("verification token count = %d, want 1", len(store.verificationTokens))
	}
	if len(store.outbox) != 1 {
		t.Fatalf("outbox count = %d, want 1", len(store.outbox))
	}
	if store.outbox[0].To != "<user@example.com>" {
		t.Fatalf("confirmation email recipient = %q, want <user@example.com>", store.outbox[0].To)
	}
	if !strings.Contains(store.outbox[0].TextBody, "http://localhost:8080/confirm-email?token=") {
		t.Fatalf("confirmation email text = %q, want confirmation URL", store.outbox[0].TextBody)
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

func TestAuthServiceChangePassword(t *testing.T) {
	service := newTestAuthService(t)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, session, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if err := service.ChangePassword(context.Background(), user, "password", "new-password"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	if _, _, err := service.Login(context.Background(), "user@example.com", "password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() old password error = %v, want %v", err, ErrInvalidCredentials)
	}

	if _, _, err := service.Login(context.Background(), "user@example.com", "new-password"); err != nil {
		t.Fatalf("Login() new password error = %v", err)
	}

	if _, err := service.UserBySessionToken(context.Background(), session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("UserBySessionToken() old session error = %v, want %v", err, ErrInvalidSession)
	}
}

func TestAuthServiceChangePasswordRejectsIncorrectCurrentPassword(t *testing.T) {
	service := newTestAuthService(t)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err = service.ChangePassword(context.Background(), user, "wrong-password", "new-password")
	if !errors.Is(err, ErrCurrentPasswordIncorrect) {
		t.Fatalf("ChangePassword() error = %v, want %v", err, ErrCurrentPasswordIncorrect)
	}
}

func TestAuthServiceChangePasswordRejectsShortPassword(t *testing.T) {
	service := newTestAuthService(t)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err = service.ChangePassword(context.Background(), user, "password", "short")
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("ChangePassword() error = %v, want %v", err, ErrInvalidPassword)
	}
}

func TestAuthServiceChangePasswordRejectsUnchangedPassword(t *testing.T) {
	service := newTestAuthService(t)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err = service.ChangePassword(context.Background(), user, "password", "password")
	if !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("ChangePassword() error = %v, want %v", err, ErrPasswordUnchanged)
	}
}

func TestAuthServiceChangePasswordWrapsStoreErrors(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	store.updateUserPasswordHashErr = errors.New("database unavailable")
	err = service.ChangePassword(context.Background(), user, "password", "new-password")
	if err == nil {
		t.Fatal("ChangePassword() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "update user password") {
		t.Fatalf("ChangePassword() error = %v, want operation context", err)
	}

	store.updateUserPasswordHashErr = nil
	store.deleteSessionsByUserIDErr = errors.New("database unavailable")
	err = service.ChangePassword(context.Background(), user, "password", "new-password")
	if err == nil {
		t.Fatal("ChangePassword() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "delete sessions by user ID") {
		t.Fatalf("ChangePassword() error = %v, want operation context", err)
	}
}

func TestAuthServiceCreateEmailVerificationToken(t *testing.T) {
	store := newFakeAuthStore()
	service := NewAuthService(store, AuthOptions{
		TokenBytes:                     32,
		EmailVerificationTokenDuration: time.Hour,
	})

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	rawToken, verificationToken, err := service.CreateEmailVerificationToken(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken() error = %v", err)
	}

	if len(rawToken) != 64 {
		t.Fatalf("raw token length = %d, want 64", len(rawToken))
	}
	if verificationToken.TokenHash == rawToken {
		t.Fatal("stored token hash equals raw token")
	}
	if verificationToken.TokenHash != hashToken(rawToken) {
		t.Fatalf("TokenHash = %q, want hash of raw token", verificationToken.TokenHash)
	}
	if time.Until(verificationToken.ExpiresAt) <= 0 {
		t.Fatalf("ExpiresAt = %s, want future time", verificationToken.ExpiresAt)
	}
}

func TestAuthServiceVerifyEmail(t *testing.T) {
	store := newFakeAuthStore()
	service := NewAuthService(store, AuthOptions{
		TokenBytes:                     32,
		EmailVerificationTokenDuration: time.Hour,
	})

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	rawToken, _, err := service.CreateEmailVerificationToken(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken() error = %v", err)
	}

	verified, err := service.VerifyEmail(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}
	if verified.ID != user.ID {
		t.Fatalf("verified user ID = %d, want %d", verified.ID, user.ID)
	}
	if !verified.EmailVerifiedAt.Valid {
		t.Fatal("EmailVerifiedAt.Valid = false, want true")
	}

	_, err = service.VerifyEmail(context.Background(), rawToken)
	if !errors.Is(err, ErrInvalidVerificationToken) {
		t.Fatalf("VerifyEmail() error = %v, want %v", err, ErrInvalidVerificationToken)
	}
}

func TestAuthServiceVerifyEmailRejectsInvalidToken(t *testing.T) {
	service := newTestAuthService(t)

	_, err := service.VerifyEmail(context.Background(), "")
	if !errors.Is(err, ErrInvalidVerificationToken) {
		t.Fatalf("VerifyEmail() error = %v, want %v", err, ErrInvalidVerificationToken)
	}

	_, err = service.VerifyEmail(context.Background(), "missing")
	if !errors.Is(err, ErrInvalidVerificationToken) {
		t.Fatalf("VerifyEmail() error = %v, want %v", err, ErrInvalidVerificationToken)
	}
}

func TestAuthServiceResendVerificationEmailForUnverifiedUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(store.outbox)

	if err := service.ResendVerificationEmail(context.Background(), user); err != nil {
		t.Fatalf("ResendVerificationEmail() error = %v", err)
	}
	if len(store.outbox) != initialOutbox+1 {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox+1)
	}
}

func TestAuthServiceResendVerificationEmailNoOpForVerifiedUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	user.EmailVerifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	initialOutbox := len(store.outbox)

	if err := service.ResendVerificationEmail(context.Background(), user); err != nil {
		t.Fatalf("ResendVerificationEmail() error = %v", err)
	}
	if len(store.outbox) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox)
	}
}

func TestAuthServiceResendVerificationEmailWrapsStoreError(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)
	store.resendErr = errors.New("database unavailable")

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	err = service.ResendVerificationEmail(context.Background(), user)
	if err == nil {
		t.Fatal("ResendVerificationEmail() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "resend email verification") {
		t.Fatalf("ResendVerificationEmail() error = %v, want operation context", err)
	}
}

func TestAuthServiceResendVerificationEmailByAddressForUnverifiedUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	if _, err := service.Register(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(store.outbox)

	if err := service.ResendVerificationEmailByAddress(context.Background(), "USER@example.com"); err != nil {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v", err)
	}
	if len(store.outbox) != initialOutbox+1 {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox+1)
	}
}

func TestAuthServiceResendVerificationEmailByAddressNoOpForMissingUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)
	initialOutbox := len(store.outbox)

	if err := service.ResendVerificationEmailByAddress(context.Background(), "missing@example.com"); err != nil {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v", err)
	}
	if len(store.outbox) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox)
	}
}

func TestAuthServiceResendVerificationEmailByAddressNoOpForVerifiedUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	user.EmailVerifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	store.usersByID[user.ID] = user
	store.usersByEmail[user.Email] = user
	initialOutbox := len(store.outbox)

	if err := service.ResendVerificationEmailByAddress(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v", err)
	}
	if len(store.outbox) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox)
	}
}

func TestAuthServiceResendVerificationEmailByAddressRejectsInvalidEmail(t *testing.T) {
	service := newTestAuthService(t)

	err := service.ResendVerificationEmailByAddress(context.Background(), "not-an-email")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v, want %v", err, ErrInvalidEmail)
	}
}

func TestAuthServiceResendVerificationEmailByAddressWrapsLookupError(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)
	store.getUserByEmailErr = errors.New("database unavailable")

	err := service.ResendVerificationEmailByAddress(context.Background(), "user@example.com")
	if err == nil {
		t.Fatal("ResendVerificationEmailByAddress() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "get user by email") {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v, want operation context", err)
	}
}

func TestAuthServiceRequestPasswordReset(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	if _, err := service.Register(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(store.outbox)

	if err := service.RequestPasswordReset(context.Background(), "USER@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if len(store.outbox) != initialOutbox+1 {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox+1)
	}
	if len(store.passwordResetTokens) != 1 {
		t.Fatalf("password reset token count = %d, want 1", len(store.passwordResetTokens))
	}
}

func TestAuthServiceRequestPasswordResetNoOpForMissingUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)
	initialOutbox := len(store.outbox)

	if err := service.RequestPasswordReset(context.Background(), "missing@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if len(store.outbox) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox)
	}
	if len(store.passwordResetTokens) != 0 {
		t.Fatalf("password reset token count = %d, want 0", len(store.passwordResetTokens))
	}
}

func TestAuthServiceRequestPasswordResetRejectsInvalidEmail(t *testing.T) {
	service := newTestAuthService(t)

	err := service.RequestPasswordReset(context.Background(), "not-an-email")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("RequestPasswordReset() error = %v, want %v", err, ErrInvalidEmail)
	}
}

func TestAuthServiceValidatePasswordResetToken(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := store.CreatePasswordResetToken(context.Background(), user.ID, hashToken("raw-token"), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("CreatePasswordResetToken() error = %v", err)
	}

	if err := service.ValidatePasswordResetToken(context.Background(), "raw-token"); err != nil {
		t.Fatalf("ValidatePasswordResetToken() error = %v", err)
	}

	if err := service.ValidatePasswordResetToken(context.Background(), ""); !errors.Is(err, ErrInvalidPasswordResetToken) {
		t.Fatalf("ValidatePasswordResetToken() error = %v, want %v", err, ErrInvalidPasswordResetToken)
	}
	if err := service.ValidatePasswordResetToken(context.Background(), "missing"); !errors.Is(err, ErrInvalidPasswordResetToken) {
		t.Fatalf("ValidatePasswordResetToken() error = %v, want %v", err, ErrInvalidPasswordResetToken)
	}
}

func TestAuthServiceResetPasswordWithToken(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	_, session, err := service.Login(context.Background(), user.Email, "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if _, err := store.CreatePasswordResetToken(context.Background(), user.ID, hashToken("raw-token"), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("CreatePasswordResetToken() error = %v", err)
	}

	if err := service.ResetPasswordWithToken(context.Background(), "raw-token", "new-password"); err != nil {
		t.Fatalf("ResetPasswordWithToken() error = %v", err)
	}
	if _, _, err := service.Login(context.Background(), user.Email, "password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() old password error = %v, want %v", err, ErrInvalidCredentials)
	}
	if _, _, err := service.Login(context.Background(), user.Email, "new-password"); err != nil {
		t.Fatalf("Login() new password error = %v", err)
	}
	if _, err := service.UserBySessionToken(context.Background(), session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("UserBySessionToken() old session error = %v, want %v", err, ErrInvalidSession)
	}
}

func TestAuthServiceResetPasswordWithTokenRejectsInvalidToken(t *testing.T) {
	service := newTestAuthService(t)

	if err := service.ResetPasswordWithToken(context.Background(), "", "new-password"); !errors.Is(err, ErrInvalidPasswordResetToken) {
		t.Fatalf("ResetPasswordWithToken() error = %v, want %v", err, ErrInvalidPasswordResetToken)
	}
	if err := service.ResetPasswordWithToken(context.Background(), "missing", "new-password"); !errors.Is(err, ErrInvalidPasswordResetToken) {
		t.Fatalf("ResetPasswordWithToken() error = %v, want %v", err, ErrInvalidPasswordResetToken)
	}
}

func TestAuthServiceResetPasswordWithTokenRejectsShortPassword(t *testing.T) {
	service := newTestAuthService(t)

	if err := service.ResetPasswordWithToken(context.Background(), "raw-token", "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("ResetPasswordWithToken() error = %v, want %v", err, ErrInvalidPassword)
	}
}

func newTestAuthService(t *testing.T) *AuthService {
	t.Helper()

	return NewAuthService(newFakeAuthStore(), AuthOptions{
		SessionDuration:     time.Hour,
		PasswordMinLen:      8,
		Argon2idMemoryKiB:   64,
		Argon2idIterations:  1,
		Argon2idParallelism: 1,
		Argon2idSaltLength:  16,
		Argon2idKeyLength:   32,
		ConfirmationEmail: email.AccountConfirmationOptions{
			AppBaseURL: "http://localhost:8080",
			From:       "Go Spark <hello@example.com>",
		},
	})
}

type fakeAuthStore struct {
	nextUserID                int64
	nextSessionID             int64
	nextVerificationTokenID   int64
	nextPasswordResetTokenID  int64
	usersByEmail              map[string]db.User
	usersByID                 map[int64]db.User
	sessions                  map[string]db.Session
	verificationTokens        map[string]db.EmailVerificationToken
	passwordResetTokens       map[string]db.PasswordResetToken
	outbox                    []email.Message
	getUserByEmailErr         error
	resendErr                 error
	updateUserPasswordHashErr error
	deleteSessionsByUserIDErr error
	passwordResetRequestErr   error
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		nextUserID:               1,
		nextSessionID:            1,
		nextVerificationTokenID:  1,
		nextPasswordResetTokenID: 1,
		usersByEmail:             make(map[string]db.User),
		usersByID:                make(map[int64]db.User),
		sessions:                 make(map[string]db.Session),
		verificationTokens:       make(map[string]db.EmailVerificationToken),
		passwordResetTokens:      make(map[string]db.PasswordResetToken),
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

func (s *fakeAuthStore) CreateUserWithEmailVerification(ctx context.Context, params CreateUserWithEmailVerificationParams) (db.User, error) {
	user, err := s.CreateUser(ctx, params.Email, params.PasswordHash)
	if err != nil {
		return db.User{}, err
	}
	if _, err := s.CreateEmailVerificationToken(ctx, user.ID, params.TokenHash, params.TokenExpiresAt); err != nil {
		delete(s.usersByEmail, user.Email)
		delete(s.usersByID, user.ID)
		return db.User{}, err
	}
	s.outbox = append(s.outbox, params.ConfirmationEmail)

	return user, nil
}

func (s *fakeAuthStore) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	if s.getUserByEmailErr != nil {
		return db.User{}, s.getUserByEmailErr
	}
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

func (s *fakeAuthStore) DeleteSessionsByUserID(ctx context.Context, userID int64) error {
	if s.deleteSessionsByUserIDErr != nil {
		return s.deleteSessionsByUserIDErr
	}

	for token, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, token)
		}
	}

	return nil
}

func (s *fakeAuthStore) UpdateUserPasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	if s.updateUserPasswordHashErr != nil {
		return s.updateUserPasswordHashErr
	}

	user, ok := s.usersByID[userID]
	if !ok {
		return sql.ErrNoRows
	}

	user.PasswordHash = passwordHash
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user

	return nil
}

func (s *fakeAuthStore) CreateEmailVerificationToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (db.EmailVerificationToken, error) {
	token := db.EmailVerificationToken{
		ID:        s.nextVerificationTokenID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	s.nextVerificationTokenID++
	s.verificationTokens[tokenHash] = token

	return token, nil
}

func (s *fakeAuthStore) CreatePasswordResetToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (db.PasswordResetToken, error) {
	token := db.PasswordResetToken{
		ID:        s.nextPasswordResetTokenID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	s.nextPasswordResetTokenID++
	s.passwordResetTokens[tokenHash] = token

	return token, nil
}

func (s *fakeAuthStore) GetValidPasswordResetTokenByHash(ctx context.Context, tokenHash string, now time.Time) (db.PasswordResetToken, error) {
	token, ok := s.passwordResetTokens[tokenHash]
	if !ok || token.ConsumedAt.Valid || !token.ExpiresAt.After(now) {
		return db.PasswordResetToken{}, sql.ErrNoRows
	}

	return token, nil
}

func (s *fakeAuthStore) ConsumePasswordResetToken(ctx context.Context, tokenHash string, consumedAt time.Time) (db.PasswordResetToken, error) {
	token, ok := s.passwordResetTokens[tokenHash]
	if !ok || token.ConsumedAt.Valid || !token.ExpiresAt.After(consumedAt) {
		return db.PasswordResetToken{}, sql.ErrNoRows
	}
	token.ConsumedAt = sql.NullTime{Time: consumedAt, Valid: true}
	s.passwordResetTokens[tokenHash] = token

	return token, nil
}

func (s *fakeAuthStore) RequestPasswordReset(ctx context.Context, params RequestPasswordResetParams) error {
	if s.passwordResetRequestErr != nil {
		return s.passwordResetRequestErr
	}
	if _, err := s.CreatePasswordResetToken(ctx, params.UserID, params.TokenHash, params.TokenExpiresAt); err != nil {
		return err
	}
	s.outbox = append(s.outbox, params.PasswordResetEmail)
	return nil
}

func (s *fakeAuthStore) VerifyEmailByTokenHash(ctx context.Context, tokenHash string, verifiedAt time.Time) (db.User, error) {
	token, ok := s.verificationTokens[tokenHash]
	if !ok || token.ConsumedAt.Valid || !token.ExpiresAt.After(verifiedAt) {
		return db.User{}, sql.ErrNoRows
	}

	user, ok := s.usersByID[token.UserID]
	if !ok {
		return db.User{}, sql.ErrNoRows
	}

	token.ConsumedAt = sql.NullTime{Time: verifiedAt, Valid: true}
	s.verificationTokens[tokenHash] = token

	user.EmailVerifiedAt = sql.NullTime{Time: verifiedAt, Valid: true}
	s.usersByID[user.ID] = user
	s.usersByEmail[user.Email] = user

	return user, nil
}

func (s *fakeAuthStore) ResendEmailVerification(ctx context.Context, params ResendEmailVerificationParams) error {
	if s.resendErr != nil {
		return s.resendErr
	}
	if _, err := s.CreateEmailVerificationToken(ctx, params.UserID, params.TokenHash, params.TokenExpiresAt); err != nil {
		return err
	}
	s.outbox = append(s.outbox, params.ConfirmationEmail)
	return nil
}
