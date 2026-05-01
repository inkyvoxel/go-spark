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

	"github.com/inkyvoxel/go-spark/internal/email"
)

const (
	DefaultPasswordMinLength             = 12
	DefaultEmailVerificationTokenTimeout = 24 * time.Hour
	DefaultPasswordResetTokenTimeout     = time.Hour
)

var (
	ErrEmailAlreadyRegistered     = errors.New("email already registered")
	ErrCurrentPasswordIncorrect   = errors.New("current password incorrect")
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrInvalidEmail               = errors.New("invalid email")
	ErrInvalidPassword            = errors.New("invalid password")
	ErrEmailUnchanged             = errors.New("email unchanged")
	ErrPasswordUnchanged          = errors.New("password unchanged")
	ErrInvalidPasswordResetToken  = errors.New("invalid password reset token")
	ErrInvalidSession             = errors.New("invalid session")
	ErrInvalidSessionTarget       = errors.New("invalid session target")
	ErrCannotRevokeCurrentSession = errors.New("cannot revoke current session")
	ErrInvalidEmailChangeToken    = errors.New("invalid email change token")
	ErrInvalidVerificationToken   = errors.New("invalid verification token")
)

type AuthService struct {
	store                          AuthStore
	emailVerificationPolicy        EmailVerificationPolicy
	emailChangeNoticeEnabled       bool
	sessionDuration                time.Duration
	emailVerificationTokenDuration time.Duration
	passwordResetTokenDuration     time.Duration
	confirmationEmail              email.AccountConfirmationOptions
	passwordResetEmail             email.PasswordResetOptions
	passwordHasher                 passwordHasher
	tokenBytes                     int
	passwordMinLen                 int
}

type AuthSession struct {
	Token     string
	ExpiresAt time.Time
}

type User struct {
	ID              int64
	Email           string
	EmailVerifiedAt sql.NullTime
	CreatedAt       time.Time
}

type UserRecord struct {
	User
	PasswordHash string
}

type SessionRecord struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type EmailVerificationToken struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	ConsumedAt sql.NullTime
	CreatedAt  time.Time
}

type PasswordResetToken struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	ConsumedAt sql.NullTime
	CreatedAt  time.Time
}

type ManagedSession struct {
	ID        int64
	CreatedAt time.Time
	ExpiresAt time.Time
	Current   bool
}

type AuthStore interface {
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	CreateVerifiedUser(ctx context.Context, email, passwordHash string, verifiedAt time.Time) (User, error)
	CreateUserWithEmailVerification(ctx context.Context, params CreateUserWithEmailVerificationParams) (User, error)
	GetUserByEmail(ctx context.Context, email string) (UserRecord, error)
	GetUserByID(ctx context.Context, userID int64) (UserRecord, error)
	CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (SessionRecord, error)
	GetUserBySessionTokenHash(ctx context.Context, tokenHash string) (UserRecord, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
	DeleteSessionsByUserID(ctx context.Context, userID int64) error
	ListActiveSessionsByUserID(ctx context.Context, userID int64) ([]SessionRecord, error)
	DeleteOtherSessionsByUserIDAndTokenHash(ctx context.Context, userID int64, tokenHash string) (int64, error)
	DeleteSessionByIDAndUserIDAndTokenHashNot(ctx context.Context, sessionID, userID int64, tokenHash string) (int64, error)
	UpdateUserPasswordHash(ctx context.Context, userID int64, passwordHash string) error
	SetPasswordAndRevokeSessions(ctx context.Context, userID int64, passwordHash string) error
	CreatePasswordResetToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (PasswordResetToken, error)
	GetValidPasswordResetTokenByHash(ctx context.Context, tokenHash string, now time.Time) (PasswordResetToken, error)
	ConsumePasswordResetToken(ctx context.Context, tokenHash string, consumedAt time.Time) (PasswordResetToken, error)
	RequestPasswordReset(ctx context.Context, params RequestPasswordResetParams) error
	RequestEmailChange(ctx context.Context, params RequestEmailChangeParams) error
	ChangeEmailImmediately(ctx context.Context, params ChangeEmailImmediatelyParams) (User, error)
	ConfirmEmailChange(ctx context.Context, params ConfirmEmailChangeParams) (User, error)
	CreateEmailVerificationToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (EmailVerificationToken, error)
	ResendEmailVerification(ctx context.Context, params ResendEmailVerificationParams) error
	VerifyEmailByTokenHash(ctx context.Context, tokenHash string, verifiedAt time.Time) (User, error)
	DeleteAccount(ctx context.Context, userID int64) error
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
	PasswordPepper                 string
	TokenBytes                     int
	PasswordMinLen                 int
	EmailVerificationTokenDuration time.Duration
	PasswordResetTokenDuration     time.Duration
	ConfirmationEmail              email.AccountConfirmationOptions
	PasswordResetEmail             email.PasswordResetOptions
	EmailVerificationPolicy        EmailVerificationPolicy
	EmailChangeNoticeEnabled       *bool
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

type RequestEmailChangeParams struct {
	UserID                 int64
	NewEmail               string
	TokenHash              string
	TokenExpiresAt         time.Time
	EmailChangeVerifyEmail email.Message
	EmailAvailableAt       time.Time
}

type ConfirmEmailChangeParams struct {
	TokenHash              string
	ChangedAt              time.Time
	OldEmailNoticeOptions  email.EmailChangeNoticeOptions
	NoticeEmailAvailableAt time.Time
	SendOldEmailNotice     bool
}

type ChangeEmailImmediatelyParams struct {
	UserID                 int64
	NewEmail               string
	ChangedAt              time.Time
	OldEmailNoticeOptions  email.EmailChangeNoticeOptions
	NoticeEmailAvailableAt time.Time
	SendOldEmailNotice     bool
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
		emailVerificationPolicy:        emailVerificationPolicy(opts.EmailVerificationPolicy),
		emailChangeNoticeEnabled:       emailChangeNoticeEnabled(opts.EmailChangeNoticeEnabled),
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

func (s *AuthService) Register(ctx context.Context, emailAddress, password string) (User, error) {
	emailAddress = normalizeEmail(emailAddress)
	if !isValidEmail(emailAddress) {
		return User{}, ErrInvalidEmail
	}
	if utf8.RuneCountInString(password) < s.passwordMinLen {
		return User{}, ErrInvalidPassword
	}
	if _, err := s.store.GetUserByEmail(ctx, emailAddress); err == nil {
		return User{}, ErrEmailAlreadyRegistered
	} else if !errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("get user by email: %w", err)
	}

	hash, err := s.passwordHasher.Hash(password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	var user User
	if s.emailVerificationPolicy.Required() {
		token, err := generateToken(s.tokenBytes)
		if err != nil {
			return User{}, fmt.Errorf("generate email verification token: %w", err)
		}

		message, err := email.NewAccountConfirmationMessage(s.confirmationEmail, emailAddress, token)
		if err != nil {
			return User{}, fmt.Errorf("build account confirmation email: %w", err)
		}

		user, err = s.store.CreateUserWithEmailVerification(
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
	} else {
		user, err = s.store.CreateVerifiedUser(ctx, emailAddress, hash, now)
	}
	if err != nil {
		if errors.Is(err, ErrEmailAlreadyRegistered) {
			return User{}, ErrEmailAlreadyRegistered
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (User, AuthSession, error) {
	user, err := s.store.GetUserByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, AuthSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, AuthSession{}, fmt.Errorf("get user by email: %w", err)
	}

	matches, err := s.passwordHasher.Verify(user.PasswordHash, password)
	if err != nil {
		return User{}, AuthSession{}, ErrInvalidCredentials
	}
	if !matches {
		return User{}, AuthSession{}, ErrInvalidCredentials
	}

	token, err := generateToken(s.tokenBytes)
	if err != nil {
		return User{}, AuthSession{}, fmt.Errorf("generate session token: %w", err)
	}

	expiresAt := time.Now().UTC().Add(s.sessionDuration)
	_, err = s.store.CreateSession(ctx, user.ID, hashToken(token), expiresAt)
	if err != nil {
		return User{}, AuthSession{}, fmt.Errorf("create session: %w", err)
	}

	return user.User, AuthSession{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *AuthService) UserBySessionToken(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrInvalidSession
	}

	user, err := s.store.GetUserBySessionTokenHash(ctx, hashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidSession
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by session token: %w", err)
	}

	return user.User, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidSession
	}

	if err := s.store.DeleteSessionByTokenHash(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (s *AuthService) ListManagedSessions(ctx context.Context, userID int64, currentSessionToken string) ([]ManagedSession, error) {
	currentSessionToken = strings.TrimSpace(currentSessionToken)
	if currentSessionToken == "" {
		return nil, ErrInvalidSession
	}

	sessions, err := s.store.ListActiveSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active sessions by user ID: %w", err)
	}

	currentTokenHash := hashToken(currentSessionToken)
	managed := make([]ManagedSession, 0, len(sessions))
	for _, session := range sessions {
		managed = append(managed, ManagedSession{
			ID:        session.ID,
			CreatedAt: session.CreatedAt,
			ExpiresAt: session.ExpiresAt,
			Current:   session.TokenHash == currentTokenHash,
		})
	}

	return managed, nil
}

func (s *AuthService) RevokeOtherSessions(ctx context.Context, userID int64, currentSessionToken string) error {
	currentSessionToken = strings.TrimSpace(currentSessionToken)
	if currentSessionToken == "" {
		return ErrInvalidSession
	}

	if _, err := s.store.DeleteOtherSessionsByUserIDAndTokenHash(ctx, userID, hashToken(currentSessionToken)); err != nil {
		return fmt.Errorf("delete other sessions by user ID: %w", err)
	}

	return nil
}

func (s *AuthService) RevokeSessionByID(ctx context.Context, userID int64, currentSessionToken string, sessionID int64) error {
	currentSessionToken = strings.TrimSpace(currentSessionToken)
	if currentSessionToken == "" {
		return ErrInvalidSession
	}
	if sessionID <= 0 {
		return ErrInvalidSessionTarget
	}

	currentTokenHash := hashToken(currentSessionToken)
	deleted, err := s.store.DeleteSessionByIDAndUserIDAndTokenHashNot(ctx, sessionID, userID, currentTokenHash)
	if err != nil {
		return fmt.Errorf("delete session by ID and user ID: %w", err)
	}
	if deleted > 0 {
		return nil
	}

	sessions, err := s.store.ListActiveSessionsByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list active sessions by user ID: %w", err)
	}
	for _, session := range sessions {
		if session.ID != sessionID {
			continue
		}
		if session.TokenHash == currentTokenHash {
			return ErrCannotRevokeCurrentSession
		}
	}

	return ErrInvalidSessionTarget
}

func (s *AuthService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCurrentPasswordIncorrect
	}
	if err != nil {
		return fmt.Errorf("get user by ID: %w", err)
	}

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

func (s *AuthService) RequestEmailChange(ctx context.Context, userID int64, currentPassword, newEmail string) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCurrentPasswordIncorrect
	}
	if err != nil {
		return fmt.Errorf("get user by ID: %w", err)
	}

	currentMatches, err := s.passwordHasher.Verify(user.PasswordHash, currentPassword)
	if err != nil || !currentMatches {
		return ErrCurrentPasswordIncorrect
	}

	newEmail = normalizeEmail(newEmail)
	if !isValidEmail(newEmail) {
		return ErrInvalidEmail
	}
	if newEmail == normalizeEmail(user.Email) {
		return ErrEmailUnchanged
	}

	existingUser, err := s.store.GetUserByEmail(ctx, newEmail)
	if err == nil && existingUser.ID != user.ID {
		return ErrEmailAlreadyRegistered
	}
	if err == nil {
		return ErrEmailUnchanged
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get user by email: %w", err)
	}

	now := time.Now().UTC()
	if s.emailVerificationPolicy.RequiresEmailChangeVerification() {
		token, err := generateToken(s.tokenBytes)
		if err != nil {
			return fmt.Errorf("generate email change token: %w", err)
		}

		message, err := email.NewEmailChangeMessage(email.EmailChangeOptions(s.confirmationEmail), newEmail, token)
		if err != nil {
			return fmt.Errorf("build email change verification email: %w", err)
		}

		if err := s.store.RequestEmailChange(ctx, RequestEmailChangeParams{
			UserID:                 user.ID,
			NewEmail:               newEmail,
			TokenHash:              hashToken(token),
			TokenExpiresAt:         now.Add(s.emailVerificationTokenDuration),
			EmailChangeVerifyEmail: message,
			EmailAvailableAt:       now,
		}); err != nil {
			return fmt.Errorf("request email change: %w", err)
		}
		return nil
	}

	if _, err := s.store.ChangeEmailImmediately(ctx, ChangeEmailImmediatelyParams{
		UserID:                 user.ID,
		NewEmail:               newEmail,
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: s.confirmationEmail.From},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     s.emailChangeNoticeEnabled,
	}); err != nil {
		if errors.Is(err, ErrEmailAlreadyRegistered) {
			return ErrEmailAlreadyRegistered
		}
		return fmt.Errorf("change email immediately: %w", err)
	}

	return nil
}

func (s *AuthService) ConfirmEmailChange(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrInvalidEmailChangeToken
	}

	now := time.Now().UTC()
	user, err := s.store.ConfirmEmailChange(ctx, ConfirmEmailChangeParams{
		TokenHash:              hashToken(token),
		ChangedAt:              now,
		OldEmailNoticeOptions:  email.EmailChangeNoticeOptions{From: s.confirmationEmail.From},
		NoticeEmailAvailableAt: now,
		SendOldEmailNotice:     s.emailChangeNoticeEnabled,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidEmailChangeToken
	}
	if err != nil {
		return User{}, fmt.Errorf("confirm email change: %w", err)
	}

	return user, nil
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
	if err := s.store.SetPasswordAndRevokeSessions(ctx, userID, hash); err != nil {
		return fmt.Errorf("set password and revoke sessions: %w", err)
	}
	return nil
}

func (s *AuthService) CreateEmailVerificationToken(ctx context.Context, userID int64) (string, EmailVerificationToken, error) {
	token, err := generateToken(s.tokenBytes)
	if err != nil {
		return "", EmailVerificationToken{}, fmt.Errorf("generate email verification token: %w", err)
	}

	verificationToken, err := s.store.CreateEmailVerificationToken(
		ctx,
		userID,
		hashToken(token),
		time.Now().UTC().Add(s.emailVerificationTokenDuration),
	)
	if err != nil {
		return "", EmailVerificationToken{}, fmt.Errorf("create email verification token: %w", err)
	}

	return token, verificationToken, nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrInvalidVerificationToken
	}

	user, err := s.store.VerifyEmailByTokenHash(ctx, hashToken(token), time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidVerificationToken
	}
	if err != nil {
		return User{}, fmt.Errorf("verify email: %w", err)
	}

	return user, nil
}

func (s *AuthService) ResendVerificationEmail(ctx context.Context, userID int64) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get user by ID: %w", err)
	}

	if !s.emailVerificationPolicy.Required() || s.emailVerificationPolicy.UserIsVerified(user.User) {
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

func (s *AuthService) DeleteAccount(ctx context.Context, userID int64, currentPassword string) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCurrentPasswordIncorrect
	}
	if err != nil {
		return fmt.Errorf("get user by ID: %w", err)
	}

	matches, err := s.passwordHasher.Verify(user.PasswordHash, currentPassword)
	if err != nil || !matches {
		return ErrCurrentPasswordIncorrect
	}

	if err := s.store.DeleteAccount(ctx, userID); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}

	return nil
}

func (s *AuthService) ResendVerificationEmailByAddress(ctx context.Context, emailAddress string) error {
	if !s.emailVerificationPolicy.Required() {
		return nil
	}

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

	if s.emailVerificationPolicy.UserIsVerified(user.User) {
		return nil
	}

	if err := s.ResendVerificationEmail(ctx, user.ID); err != nil {
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

func emailVerificationPolicy(policy EmailVerificationPolicy) EmailVerificationPolicy {
	if policy == nil {
		return DefaultEmailVerificationPolicy()
	}
	return policy
}

func emailChangeNoticeEnabled(enabled *bool) bool {
	if enabled == nil {
		return true
	}
	return *enabled
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
