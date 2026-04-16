package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
)

const (
	DefaultPasswordMinLength             = 12
	DefaultEmailVerificationTokenTimeout = 24 * time.Hour
	DefaultPasswordResetTokenTimeout     = time.Hour
)

var (
	ErrEmailAlreadyRegistered    = errors.New("email already registered")
	ErrCurrentPasswordIncorrect  = errors.New("current password incorrect")
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrInvalidEmail              = errors.New("invalid email")
	ErrInvalidPassword           = errors.New("invalid password")
	ErrPasswordUnchanged         = errors.New("password unchanged")
	ErrInvalidPasswordResetToken = errors.New("invalid password reset token")
	ErrInvalidSession            = errors.New("invalid session")
	ErrInvalidVerificationToken  = errors.New("invalid verification token")
)

type AuthService struct {
	store                          AuthStore
	sessionDuration                time.Duration
	emailVerificationTokenDuration time.Duration
	passwordResetTokenDuration     time.Duration
	confirmationEmail              email.AccountConfirmationOptions
	passwordResetEmail             email.PasswordResetOptions
	passwordHasher                 passwordHasher
	tokenBytes                     int
	passwordMinLen                 int
}

type AuthStore interface {
	CreateUser(ctx context.Context, email, passwordHash string) (db.User, error)
	CreateUserWithEmailVerification(ctx context.Context, params CreateUserWithEmailVerificationParams) (db.User, error)
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
	CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) (db.Session, error)
	GetUserBySessionToken(ctx context.Context, token string) (db.User, error)
	DeleteSessionByToken(ctx context.Context, token string) error
	DeleteSessionsByUserID(ctx context.Context, userID int64) error
	UpdateUserPasswordHash(ctx context.Context, userID int64, passwordHash string) error
	CreatePasswordResetToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (db.PasswordResetToken, error)
	GetValidPasswordResetTokenByHash(ctx context.Context, tokenHash string, now time.Time) (db.PasswordResetToken, error)
	ConsumePasswordResetToken(ctx context.Context, tokenHash string, consumedAt time.Time) (db.PasswordResetToken, error)
	RequestPasswordReset(ctx context.Context, params RequestPasswordResetParams) error
	CreateEmailVerificationToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (db.EmailVerificationToken, error)
	ResendEmailVerification(ctx context.Context, params ResendEmailVerificationParams) error
	VerifyEmailByTokenHash(ctx context.Context, tokenHash string, verifiedAt time.Time) (db.User, error)
}

type CreateUserWithEmailVerificationParams struct {
	Email             string
	PasswordHash      string
	TokenHash         string
	TokenExpiresAt    time.Time
	ConfirmationEmail email.Message
	EmailAvailableAt  time.Time
}

type AuthOptions struct {
	SessionDuration                time.Duration
	Argon2idMemoryKiB              uint32
	Argon2idIterations             uint32
	Argon2idParallelism            uint8
	Argon2idSaltLength             uint32
	Argon2idKeyLength              uint32
	TokenBytes                     int
	PasswordMinLen                 int
	EmailVerificationTokenDuration time.Duration
	PasswordResetTokenDuration     time.Duration
	ConfirmationEmail              email.AccountConfirmationOptions
	PasswordResetEmail             email.PasswordResetOptions
}

type ResendEmailVerificationParams struct {
	UserID            int64
	TokenHash         string
	TokenExpiresAt    time.Time
	ConfirmationEmail email.Message
	EmailAvailableAt  time.Time
}

type RequestPasswordResetParams struct {
	UserID             int64
	TokenHash          string
	TokenExpiresAt     time.Time
	PasswordResetEmail email.Message
	EmailAvailableAt   time.Time
}

func NewAuthService(store AuthStore, opts AuthOptions) *AuthService {
	sessionDuration := opts.SessionDuration
	if sessionDuration == 0 {
		sessionDuration = 7 * 24 * time.Hour
	}

	tokenBytes := opts.TokenBytes
	if tokenBytes == 0 {
		tokenBytes = 32
	}

	passwordMinLen := opts.PasswordMinLen
	if passwordMinLen == 0 {
		passwordMinLen = DefaultPasswordMinLength
	}

	emailVerificationTokenDuration := opts.EmailVerificationTokenDuration
	if emailVerificationTokenDuration == 0 {
		emailVerificationTokenDuration = DefaultEmailVerificationTokenTimeout
	}

	passwordResetTokenDuration := opts.PasswordResetTokenDuration
	if passwordResetTokenDuration == 0 {
		passwordResetTokenDuration = DefaultPasswordResetTokenTimeout
	}

	passwordResetEmail := opts.PasswordResetEmail
	if strings.TrimSpace(passwordResetEmail.AppBaseURL) == "" {
		passwordResetEmail.AppBaseURL = opts.ConfirmationEmail.AppBaseURL
	}
	if strings.TrimSpace(passwordResetEmail.From) == "" {
		passwordResetEmail.From = opts.ConfirmationEmail.From
	}

	return &AuthService{
		store:                          store,
		sessionDuration:                sessionDuration,
		emailVerificationTokenDuration: emailVerificationTokenDuration,
		passwordResetTokenDuration:     passwordResetTokenDuration,
		confirmationEmail:              opts.ConfirmationEmail,
		passwordResetEmail:             passwordResetEmail,
		passwordHasher:                 newArgon2idHasher(opts),
		tokenBytes:                     tokenBytes,
		passwordMinLen:                 passwordMinLen,
	}
}

func (s *AuthService) Register(ctx context.Context, emailAddress, password string) (db.User, error) {
	emailAddress = normalizeEmail(emailAddress)
	if !isValidEmail(emailAddress) {
		return db.User{}, ErrInvalidEmail
	}
	if utf8.RuneCountInString(password) < s.passwordMinLen {
		return db.User{}, ErrInvalidPassword
	}

	hash, err := s.passwordHasher.Hash(password)
	if err != nil {
		return db.User{}, fmt.Errorf("hash password: %w", err)
	}

	token, err := generateToken(s.tokenBytes)
	if err != nil {
		return db.User{}, fmt.Errorf("generate email verification token: %w", err)
	}

	message, err := email.NewAccountConfirmationMessage(s.confirmationEmail, emailAddress, token)
	if err != nil {
		return db.User{}, fmt.Errorf("build account confirmation email: %w", err)
	}

	now := time.Now().UTC()
	user, err := s.store.CreateUserWithEmailVerification(
		ctx,
		CreateUserWithEmailVerificationParams{
			Email:             emailAddress,
			PasswordHash:      hash,
			TokenHash:         hashToken(token),
			TokenExpiresAt:    now.Add(s.emailVerificationTokenDuration),
			ConfirmationEmail: message,
			EmailAvailableAt:  now,
		},
	)
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

	matches, err := s.passwordHasher.Verify(user.PasswordHash, password)
	if err != nil {
		return db.User{}, db.Session{}, ErrInvalidCredentials
	}
	if !matches {
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

func (s *AuthService) ChangePassword(ctx context.Context, user db.User, currentPassword, newPassword string) error {
	currentMatches, err := s.passwordHasher.Verify(user.PasswordHash, currentPassword)
	if err != nil || !currentMatches {
		return ErrCurrentPasswordIncorrect
	}

	if err := s.validatePassword(newPassword); err != nil {
		return err
	}

	unchanged, err := s.passwordHasher.Verify(user.PasswordHash, newPassword)
	if err == nil && unchanged {
		return ErrPasswordUnchanged
	}

	if err := s.setPasswordAndRevokeSessions(ctx, user.ID, newPassword); err != nil {
		return err
	}

	return nil
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, emailAddress string) error {
	emailAddress = normalizeEmail(emailAddress)
	if !isValidEmail(emailAddress) {
		return ErrInvalidEmail
	}

	user, err := s.store.GetUserByEmail(ctx, emailAddress)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get user by email: %w", err)
	}

	token, err := generateToken(s.tokenBytes)
	if err != nil {
		return fmt.Errorf("generate password reset token: %w", err)
	}

	message, err := email.NewPasswordResetMessage(s.passwordResetEmail, user.Email, token)
	if err != nil {
		return fmt.Errorf("build password reset email: %w", err)
	}

	now := time.Now().UTC()
	if err := s.store.RequestPasswordReset(ctx, RequestPasswordResetParams{
		UserID:             user.ID,
		TokenHash:          hashToken(token),
		TokenExpiresAt:     now.Add(s.passwordResetTokenDuration),
		PasswordResetEmail: message,
		EmailAvailableAt:   now,
	}); err != nil {
		return fmt.Errorf("request password reset: %w", err)
	}

	return nil
}

func (s *AuthService) ValidatePasswordResetToken(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidPasswordResetToken
	}

	_, err := s.store.GetValidPasswordResetTokenByHash(ctx, hashToken(token), time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidPasswordResetToken
	}
	if err != nil {
		return fmt.Errorf("get password reset token: %w", err)
	}

	return nil
}

func (s *AuthService) ResetPasswordWithToken(ctx context.Context, token, newPassword string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidPasswordResetToken
	}

	if err := s.validatePassword(newPassword); err != nil {
		return err
	}

	resetToken, err := s.store.ConsumePasswordResetToken(ctx, hashToken(token), time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidPasswordResetToken
	}
	if err != nil {
		return fmt.Errorf("consume password reset token: %w", err)
	}

	if err := s.setPasswordAndRevokeSessions(ctx, resetToken.UserID, newPassword); err != nil {
		return err
	}

	return nil
}

func (s *AuthService) validatePassword(password string) error {
	if utf8.RuneCountInString(password) < s.passwordMinLen {
		return ErrInvalidPassword
	}

	return nil
}

func (s *AuthService) setPasswordAndRevokeSessions(ctx context.Context, userID int64, newPassword string) error {
	hash, err := s.passwordHasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.store.UpdateUserPasswordHash(ctx, userID, hash); err != nil {
		return fmt.Errorf("update user password: %w", err)
	}

	if err := s.store.DeleteSessionsByUserID(ctx, userID); err != nil {
		return fmt.Errorf("delete sessions by user ID: %w", err)
	}
	return nil
}

func (s *AuthService) CreateEmailVerificationToken(ctx context.Context, userID int64) (string, db.EmailVerificationToken, error) {
	token, err := generateToken(s.tokenBytes)
	if err != nil {
		return "", db.EmailVerificationToken{}, fmt.Errorf("generate email verification token: %w", err)
	}

	verificationToken, err := s.store.CreateEmailVerificationToken(
		ctx,
		userID,
		hashToken(token),
		time.Now().UTC().Add(s.emailVerificationTokenDuration),
	)
	if err != nil {
		return "", db.EmailVerificationToken{}, fmt.Errorf("create email verification token: %w", err)
	}

	return token, verificationToken, nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, token string) (db.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return db.User{}, ErrInvalidVerificationToken
	}

	user, err := s.store.VerifyEmailByTokenHash(ctx, hashToken(token), time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		return db.User{}, ErrInvalidVerificationToken
	}
	if err != nil {
		return db.User{}, fmt.Errorf("verify email: %w", err)
	}

	return user, nil
}

func (s *AuthService) ResendVerificationEmail(ctx context.Context, user db.User) error {
	if user.EmailVerifiedAt.Valid {
		return nil
	}

	token, err := generateToken(s.tokenBytes)
	if err != nil {
		return fmt.Errorf("generate email verification token: %w", err)
	}

	message, err := email.NewAccountConfirmationMessage(s.confirmationEmail, user.Email, token)
	if err != nil {
		return fmt.Errorf("build account confirmation email: %w", err)
	}

	now := time.Now().UTC()
	if err := s.store.ResendEmailVerification(ctx, ResendEmailVerificationParams{
		UserID:            user.ID,
		TokenHash:         hashToken(token),
		TokenExpiresAt:    now.Add(s.emailVerificationTokenDuration),
		ConfirmationEmail: message,
		EmailAvailableAt:  now,
	}); err != nil {
		return fmt.Errorf("resend email verification: %w", err)
	}

	return nil
}

func (s *AuthService) ResendVerificationEmailByAddress(ctx context.Context, emailAddress string) error {
	emailAddress = normalizeEmail(emailAddress)
	if !isValidEmail(emailAddress) {
		return ErrInvalidEmail
	}

	user, err := s.store.GetUserByEmail(ctx, emailAddress)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get user by email: %w", err)
	}

	if user.EmailVerifiedAt.Valid {
		return nil
	}

	if err := s.ResendVerificationEmail(ctx, user); err != nil {
		return err
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

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
