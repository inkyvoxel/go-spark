package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/sqlitetest"
	"github.com/inkyvoxel/go-spark/internal/totp"
)

func TestAuthServiceBeginTOTPSetupUsesConfiguredIssuer(t *testing.T) {
	service := mustNewAuthService(t, AuthOptions{TOTPIssuer: "Acme Corp"})
	store := service.store

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	_, uri, err := service.BeginTOTPSetup(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("BeginTOTPSetup() error = %v", err)
	}

	if !strings.Contains(uri, "Acme+Corp") && !strings.Contains(uri, "Acme%20Corp") && !strings.Contains(uri, "Acme Corp") {
		t.Fatalf("otpauth URI = %q, want issuer %q in URI", uri, "Acme Corp")
	}
}

func TestBeginTOTPSetup_WhenAlreadyEnabled_ReturnsError(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, _, err := service.BeginTOTPSetup(context.Background(), user.ID); err != nil {
		t.Fatalf("BeginTOTPSetup() first call error = %v", err)
	}
	if codes, err := service.ConfirmTOTPSetup(context.Background(), user.ID, validTOTPCode(t, store, user.ID)); err != nil || len(codes) == 0 {
		t.Fatalf("ConfirmTOTPSetup() error = %v, codes = %v", err, codes)
	}

	_, _, err = service.BeginTOTPSetup(context.Background(), user.ID)
	if !errors.Is(err, ErrTOTPAlreadyEnabled) {
		t.Fatalf("BeginTOTPSetup() when already enabled = %v, want ErrTOTPAlreadyEnabled", err)
	}
}

func TestBeginTOTPSetup_WhenAlreadyEnabled_DoesNotClearEnabledAt(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, _, err := service.BeginTOTPSetup(context.Background(), user.ID); err != nil {
		t.Fatalf("BeginTOTPSetup() first call error = %v", err)
	}
	if codes, err := service.ConfirmTOTPSetup(context.Background(), user.ID, validTOTPCode(t, store, user.ID)); err != nil || len(codes) == 0 {
		t.Fatalf("ConfirmTOTPSetup() error = %v, codes = %v", err, codes)
	}

	before, err := store.GetTOTPByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetTOTPByUserID() before error = %v", err)
	}
	service.BeginTOTPSetup(context.Background(), user.ID) //nolint:errcheck
	after, err := store.GetTOTPByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetTOTPByUserID() after error = %v", err)
	}

	if after.Secret != before.Secret {
		t.Errorf("Secret changed after rejected setup: got %q, want %q", after.Secret, before.Secret)
	}
	if !after.EnabledAt.Valid || after.EnabledAt.Time != before.EnabledAt.Time {
		t.Errorf("EnabledAt changed after rejected setup: got %v, want %v", after.EnabledAt, before.EnabledAt)
	}
}

// validTOTPCode generates a valid TOTP code for the pending secret stored for the user.
func validTOTPCode(t *testing.T, store *authStore, userID int64) string {
	t.Helper()
	record, err := store.GetTOTPByUserID(context.Background(), userID)
	if err != nil {
		t.Fatalf("no TOTP record found for user: %v", err)
	}
	code, err := totp.Generate(record.Secret)
	if err != nil {
		t.Fatalf("totp.Generate() error = %v", err)
	}
	return code
}

// enableTOTP sets up and confirms TOTP for the given user, returning the plaintext backup codes.
func enableTOTP(t *testing.T, service *AuthService, store *authStore, userID int64) []string {
	t.Helper()
	if _, _, err := service.BeginTOTPSetup(context.Background(), userID); err != nil {
		t.Fatalf("BeginTOTPSetup() error = %v", err)
	}
	codes, err := service.ConfirmTOTPSetup(context.Background(), userID, validTOTPCode(t, store, userID))
	if err != nil {
		t.Fatalf("ConfirmTOTPSetup() error = %v", err)
	}
	return codes
}

// resetTOTPReplayGuard clears the stored last-used counter, simulating the
// passage of time into a new TOTP window so a fresh code from the current
// window can be accepted after an earlier verification consumed it.
func resetTOTPReplayGuard(t *testing.T, store *authStore, userID int64) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(),
		`UPDATE user_totp SET last_used_counter = NULL WHERE user_id = ?`, userID); err != nil {
		t.Fatalf("reset TOTP replay guard: %v", err)
	}
}

func TestVerifyTOTPLogin_ReplayedCode_Fails(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	enableTOTP(t, service, store, user.ID)
	resetTOTPReplayGuard(t, store, user.ID)

	code := validTOTPCode(t, store, user.ID)
	if _, err := service.VerifyTOTPLogin(context.Background(), user.ID, code); err != nil {
		t.Fatalf("VerifyTOTPLogin() first use error = %v, want nil", err)
	}

	_, err = service.VerifyTOTPLogin(context.Background(), user.ID, code)
	if !errors.Is(err, ErrInvalidTOTPCode) {
		t.Fatalf("VerifyTOTPLogin() replayed code = %v, want ErrInvalidTOTPCode", err)
	}
}

func TestVerifyTOTPLogin_WithBackupCode_Succeeds(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	codes := enableTOTP(t, service, store, user.ID)

	_, err = service.VerifyTOTPLogin(context.Background(), user.ID, codes[0])
	if err != nil {
		t.Fatalf("VerifyTOTPLogin() with backup code error = %v, want nil", err)
	}
}

func TestVerifyTOTPLogin_WithUsedBackupCode_Fails(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	codes := enableTOTP(t, service, store, user.ID)

	if _, err := service.VerifyTOTPLogin(context.Background(), user.ID, codes[0]); err != nil {
		t.Fatalf("VerifyTOTPLogin() first use error = %v", err)
	}

	_, err = service.VerifyTOTPLogin(context.Background(), user.ID, codes[0])
	if !errors.Is(err, ErrInvalidTOTPCode) {
		t.Fatalf("VerifyTOTPLogin() with used backup code = %v, want ErrInvalidTOTPCode", err)
	}
}

func TestDisableTOTP_WithBackupCode_Succeeds(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	codes := enableTOTP(t, service, store, user.ID)

	if err := service.DisableTOTP(context.Background(), user.ID, codes[0]); err != nil {
		t.Fatalf("DisableTOTP() with backup code error = %v, want nil", err)
	}

	enabled, _, err := service.GetTOTPStatus(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetTOTPStatus() error = %v", err)
	}
	if enabled {
		t.Fatal("GetTOTPStatus() enabled = true, want false after disable")
	}
}

func TestDisableTOTP_WithInvalidCode_Fails(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	enableTOTP(t, service, store, user.ID)

	err = service.DisableTOTP(context.Background(), user.ID, "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX")
	if !errors.Is(err, ErrInvalidTOTPCode) {
		t.Fatalf("DisableTOTP() with invalid code = %v, want ErrInvalidTOTPCode", err)
	}
}

func TestRegenerateBackupCodes_WithTOTPCode_ReturnsNewCodes(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	originalCodes := enableTOTP(t, service, store, user.ID)
	resetTOTPReplayGuard(t, store, user.ID)

	newCodes, err := service.RegenerateBackupCodes(context.Background(), user.ID, validTOTPCode(t, store, user.ID))
	if err != nil {
		t.Fatalf("RegenerateBackupCodes() error = %v, want nil", err)
	}
	if len(newCodes) != totpBackupCodeCount {
		t.Fatalf("len(newCodes) = %d, want %d", len(newCodes), totpBackupCodeCount)
	}

	// Old codes must no longer work.
	_, err = service.VerifyTOTPLogin(context.Background(), user.ID, originalCodes[0])
	if !errors.Is(err, ErrInvalidTOTPCode) {
		t.Fatalf("VerifyTOTPLogin() with old backup code = %v, want ErrInvalidTOTPCode", err)
	}
}

func TestRegenerateBackupCodes_WithBackupCode_Succeeds(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	codes := enableTOTP(t, service, store, user.ID)

	_, err = service.RegenerateBackupCodes(context.Background(), user.ID, codes[0])
	if err != nil {
		t.Fatalf("RegenerateBackupCodes() with backup code error = %v, want nil", err)
	}
}

func TestRegenerateBackupCodes_WithInvalidCode_Fails(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	enableTOTP(t, service, store, user.ID)

	_, err = service.RegenerateBackupCodes(context.Background(), user.ID, "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX")
	if !errors.Is(err, ErrInvalidTOTPCode) {
		t.Fatalf("RegenerateBackupCodes() with invalid code = %v, want ErrInvalidTOTPCode", err)
	}
}

func TestRegenerateBackupCodes_WhenTOTPNotEnabled_Fails(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}

	_, err = service.RegenerateBackupCodes(context.Background(), user.ID, "000000")
	if !errors.Is(err, ErrTOTPNotEnabled) {
		t.Fatalf("RegenerateBackupCodes() when TOTP not enabled = %v, want ErrTOTPNotEnabled", err)
	}
}

func TestConfirmTOTPSetup_BackupCodesHaveExpectedFormat(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := store.CreateVerifiedUser(context.Background(), "user@example.com", "hash", time.Now())
	if err != nil {
		t.Fatalf("CreateVerifiedUser() error = %v", err)
	}
	codes := enableTOTP(t, service, store, user.ID)

	if len(codes) != totpBackupCodeCount {
		t.Fatalf("len(codes) = %d, want %d", len(codes), totpBackupCodeCount)
	}
	for _, code := range codes {
		// Expected format: XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX (32 hex chars + 3 dashes = 35 chars)
		if len(code) != 35 {
			t.Errorf("code %q has length %d, want 35", code, len(code))
		}
		parts := strings.Split(code, "-")
		if len(parts) != 4 {
			t.Errorf("code %q has %d dash-separated parts, want 4", code, len(parts))
			continue
		}
		for _, part := range parts {
			if len(part) != 8 {
				t.Errorf("code %q: part %q has length %d, want 8", code, part, len(part))
			}
		}
	}
}

func TestAuthServiceRegisterHashesPassword(t *testing.T) {
	service := newTestAuthService(t)

	user, err := service.Register(context.Background(), "  USER@example.COM  ", "correct horse battery staple")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if user.Email != "user@example.com" {
		t.Fatalf("Email = %q, want %q", user.Email, "user@example.com")
	}
	store := service.store
	storedUser, err := store.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if storedUser.PasswordHash == "correct horse battery staple" {
		t.Fatal("PasswordHash stores plaintext password")
	}
	matches, err := service.passwordHasher.Verify(storedUser.PasswordHash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !matches {
		t.Fatal("Verify() = false, want true")
	}

	if countRows(t, store, "email_verification_tokens") != 1 {
		t.Fatalf("verification token count = %d, want 1", countRows(t, store, "email_verification_tokens"))
	}
	if len(outbox(t, store)) != 1 {
		t.Fatalf("outbox count = %d, want 1", len(outbox(t, store)))
	}
	if outbox(t, store)[0].To != "<user@example.com>" {
		t.Fatalf("confirmation email recipient = %q, want <user@example.com>", outbox(t, store)[0].To)
	}
	if !strings.Contains(outbox(t, store)[0].TextBody, "http://localhost:8080"+paths.ConfirmEmail+"?token=") {
		t.Fatalf("confirmation email text = %q, want confirmation URL", outbox(t, store)[0].TextBody)
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
	tooLong := strings.Repeat("a", PasswordMaxLength+1)
	if _, err := service.Register(context.Background(), "user@example.com", tooLong); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("Register() error = %v, want %v", err, ErrPasswordTooLong)
	}
}

func TestAuthServiceRegisterDuplicateEmailIsNeutral(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	if _, err := service.Register(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Registering an existing address must not reveal that it is taken: it
	// returns the same nil error as a genuine signup, but creates no second
	// account and enqueues no further email.
	user, err := service.Register(context.Background(), "USER@example.com", "password")
	if err != nil {
		t.Fatalf("Register() duplicate error = %v, want nil", err)
	}
	if user.ID != 0 {
		t.Fatalf("Register() duplicate user = %+v, want zero user", user)
	}
	if countRows(t, store, "users") != 1 {
		t.Fatalf("user count = %d, want 1 (no duplicate created)", countRows(t, store, "users"))
	}
	if len(outbox(t, store)) != 1 {
		t.Fatalf("outbox count = %d, want 1 (no second confirmation email)", len(outbox(t, store)))
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
	if len(session.Token) != 64 {
		t.Fatalf("session token length = %d, want %d", len(session.Token), 64)
	}
	if time.Until(session.ExpiresAt) <= 0 {
		t.Fatalf("session ExpiresAt = %s, want future time", session.ExpiresAt)
	}
	store := service.store
	if sessionExists(t, store, session.Token) {
		t.Fatal("raw session token stored, want hashed-only storage")
	}
	if !sessionExists(t, store, hashToken(session.Token)) {
		t.Fatal("session hash not found in store")
	}
}

func TestAuthServiceWithPepperSupportsRegisterLoginAndPasswordChange(t *testing.T) {
	service := mustNewAuthService(t, AuthOptions{
		SessionDuration:     time.Hour,
		PasswordMinLen:      8,
		Argon2idMemoryKiB:   64,
		Argon2idIterations:  1,
		Argon2idParallelism: 1,
		PasswordPepper:      "test-pepper",
		EmailOptions: email.MessageOptions{
			AppBaseURL: "http://localhost:8080",
			From:       "Go Spark <hello@example.com>",
		},
	})

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if _, _, err := service.Login(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if err := service.ChangePassword(context.Background(), user.ID, "password", "new-password"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	if _, _, err := service.Login(context.Background(), "user@example.com", "password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() old password error = %v, want %v", err, ErrInvalidCredentials)
	}
	if _, _, err := service.Login(context.Background(), "user@example.com", "new-password"); err != nil {
		t.Fatalf("Login() new password error = %v", err)
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

func TestAuthServiceListManagedSessionsAndRevokeControls(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, currentSession, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	_, otherSession, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	currentStoreSessionID := sessionID(t, store, hashToken(currentSession.Token))
	otherStoreSessionID := sessionID(t, store, hashToken(otherSession.Token))

	managed, err := service.ListManagedSessions(context.Background(), user.ID, currentSession.Token)
	if err != nil {
		t.Fatalf("ListManagedSessions() error = %v", err)
	}
	if len(managed) != 2 {
		t.Fatalf("managed session count = %d, want %d", len(managed), 2)
	}
	var currentCount int
	for _, session := range managed {
		if session.Current {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("current session count = %d, want 1", currentCount)
	}

	if err := service.RevokeSessionByID(context.Background(), user.ID, currentSession.Token, currentStoreSessionID); !errors.Is(err, ErrCannotRevokeCurrentSession) {
		t.Fatalf("RevokeSessionByID(current) error = %v, want %v", err, ErrCannotRevokeCurrentSession)
	}

	if err := service.RevokeSessionByID(context.Background(), user.ID, currentSession.Token, otherStoreSessionID); err != nil {
		t.Fatalf("RevokeSessionByID(other) error = %v", err)
	}
	if sessionExists(t, store, hashToken(otherSession.Token)) {
		t.Fatal("other session still present after revoke")
	}

	_, anotherSession, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := service.RevokeOtherSessions(context.Background(), user.ID, currentSession.Token); err != nil {
		t.Fatalf("RevokeOtherSessions() error = %v", err)
	}
	if !sessionExists(t, store, hashToken(currentSession.Token)) {
		t.Fatal("current session missing after revoke others")
	}
	if sessionExists(t, store, hashToken(anotherSession.Token)) {
		t.Fatal("other session still present after revoke others")
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

	if err := service.ChangePassword(context.Background(), user.ID, "password", "new-password"); err != nil {
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

	err = service.ChangePassword(context.Background(), user.ID, "wrong-password", "new-password")
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

	err = service.ChangePassword(context.Background(), user.ID, "password", "short")
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

	err = service.ChangePassword(context.Background(), user.ID, "password", "password")
	if !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("ChangePassword() error = %v, want %v", err, ErrPasswordUnchanged)
	}
}

func TestAuthServiceRequestEmailChange(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := service.RequestEmailChange(context.Background(), user.ID, "password", "NEW@example.com"); err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}

	if countRows(t, store, "email_change_tokens") != 1 {
		t.Fatalf("email change token count = %d, want 1", countRows(t, store, "email_change_tokens"))
	}
	token := singleEmailChangeToken(t, store)
	if token.UserID != user.ID {
		t.Fatalf("email change token user ID = %d, want %d", token.UserID, user.ID)
	}
	if token.NewEmail != "new@example.com" {
		t.Fatalf("email change token new email = %q, want normalized email", token.NewEmail)
	}
	if len(outbox(t, store)) != 2 {
		t.Fatalf("outbox count = %d, want registration and email-change messages", len(outbox(t, store)))
	}
	message := outbox(t, store)[len(outbox(t, store))-1]
	if message.To != "<new@example.com>" {
		t.Fatalf("email change To = %q, want new email", message.To)
	}
	if !strings.Contains(message.TextBody, "/account/confirm-email-change?token=") {
		t.Fatalf("email change TextBody = %q, want confirmation link", message.TextBody)
	}
}

func TestAuthServiceRequestEmailChangeRejectsInvalidInputs(t *testing.T) {
	service := newTestAuthService(t)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := service.Register(context.Background(), "taken@example.com", "password"); err != nil {
		t.Fatalf("Register() taken error = %v", err)
	}

	tests := []struct {
		name     string
		password string
		email    string
		want     error
	}{
		{name: "incorrect current password", password: "wrong-password", email: "new@example.com", want: ErrCurrentPasswordIncorrect},
		{name: "invalid email", password: "password", email: "not-an-email", want: ErrInvalidEmail},
		{name: "unchanged email", password: "password", email: "USER@example.com", want: ErrEmailUnchanged},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.RequestEmailChange(context.Background(), user.ID, tt.password, tt.email)
			if !errors.Is(err, tt.want) {
				t.Fatalf("RequestEmailChange() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAuthServiceRequestEmailChangeTakenEmailIsNeutral(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := service.Register(context.Background(), "taken@example.com", "password"); err != nil {
		t.Fatalf("Register() taken error = %v", err)
	}

	outboxBefore := len(outbox(t, store))
	tokensBefore := countRows(t, store, "email_change_tokens")

	// Requesting a change to an address owned by another account must look
	// identical to a successful request (nil error), but it must never persist
	// a token or send a verification link to that address.
	if err := service.RequestEmailChange(context.Background(), user.ID, "password", "taken@example.com"); err != nil {
		t.Fatalf("RequestEmailChange() taken error = %v, want nil", err)
	}
	if len(outbox(t, store)) != outboxBefore {
		t.Fatalf("outbox count = %d, want %d (no email sent for taken address)", len(outbox(t, store)), outboxBefore)
	}
	if countRows(t, store, "email_change_tokens") != tokensBefore {
		t.Fatalf("email change token count = %d, want %d (no token persisted)", countRows(t, store, "email_change_tokens"), tokensBefore)
	}
}

func TestAuthServiceConfirmEmailChange(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	_, session, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	seedEmailChangeToken(t, store, user.ID, "new@example.com", hashToken("raw-token"), time.Now().UTC().Add(time.Hour))

	changed, err := service.ConfirmEmailChange(context.Background(), "raw-token")
	if err != nil {
		t.Fatalf("ConfirmEmailChange() error = %v", err)
	}
	if changed.Email != "new@example.com" {
		t.Fatalf("changed email = %q, want new email", changed.Email)
	}
	if !changed.EmailVerifiedAt.Valid {
		t.Fatal("changed EmailVerifiedAt.Valid = false, want true")
	}
	if _, err := store.GetUserByEmail(context.Background(), "user@example.com"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old email lookup error = %v, want %v", err, sql.ErrNoRows)
	}
	if _, err := store.GetUserByEmail(context.Background(), "new@example.com"); err != nil {
		t.Fatalf("new email lookup error = %v", err)
	}
	if _, err := service.UserBySessionToken(context.Background(), session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old session error = %v, want %v", err, ErrInvalidSession)
	}
	notice := outbox(t, store)[len(outbox(t, store))-1]
	if notice.To != "<user@example.com>" {
		t.Fatalf("old email notice To = %q, want old email", notice.To)
	}
	if notice.Subject != "Your email address was changed" {
		t.Fatalf("old email notice subject = %q, want change notice", notice.Subject)
	}
}

func TestAuthServiceConfirmEmailChangeRejectsInvalidToken(t *testing.T) {
	service := newTestAuthService(t)

	for _, token := range []string{"", "missing"} {
		if _, err := service.ConfirmEmailChange(context.Background(), token); !errors.Is(err, ErrInvalidEmailChangeToken) {
			t.Fatalf("ConfirmEmailChange(%q) error = %v, want %v", token, err, ErrInvalidEmailChangeToken)
		}
	}
}

func TestAuthServiceConfirmEmailChangeRejectsExpiredToken(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	seedEmailChangeToken(t, store, user.ID, "new@example.com", hashToken("raw-token"), time.Now().UTC().Add(-time.Minute))

	_, err = service.ConfirmEmailChange(context.Background(), "raw-token")
	if !errors.Is(err, ErrInvalidEmailChangeToken) {
		t.Fatalf("ConfirmEmailChange() error = %v, want %v", err, ErrInvalidEmailChangeToken)
	}
}

func TestAuthServiceConfirmEmailChangeRejectsAlreadyOwnedEmail(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := service.Register(context.Background(), "new@example.com", "password"); err != nil {
		t.Fatalf("Register() competing email error = %v", err)
	}
	seedEmailChangeToken(t, store, user.ID, "new@example.com", hashToken("raw-token"), time.Now().UTC().Add(time.Hour))

	_, err = service.ConfirmEmailChange(context.Background(), "raw-token")
	if !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("ConfirmEmailChange() error = %v, want %v", err, ErrEmailAlreadyRegistered)
	}
}

func TestAuthServiceConfirmEmailChangeSkipsOldEmailNoticeWhenDisabled(t *testing.T) {
	service := newTestAuthServiceWithNoticeDisabled(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(outbox(t, store))
	seedEmailChangeToken(t, store, user.ID, "new@example.com", hashToken("raw-token"), time.Now().UTC().Add(time.Hour))

	if _, err := service.ConfirmEmailChange(context.Background(), "raw-token"); err != nil {
		t.Fatalf("ConfirmEmailChange() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox)
	}
}

func TestAuthServiceCreateEmailVerificationToken(t *testing.T) {
	service := mustNewAuthService(t, AuthOptions{
		TokenBytes:                     32,
		EmailVerificationTokenDuration: time.Hour,
	})
	store := service.store

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
	service := mustNewAuthService(t, AuthOptions{
		TokenBytes:                     32,
		EmailVerificationTokenDuration: time.Hour,
	})
	store := service.store

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
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(outbox(t, store))

	if err := service.ResendVerificationEmail(context.Background(), user.ID); err != nil {
		t.Fatalf("ResendVerificationEmail() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox+1 {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox+1)
	}
}

func TestAuthServiceResendVerificationEmailNoOpForVerifiedUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	markUserVerified(t, store, user.ID)
	initialOutbox := len(outbox(t, store))

	if err := service.ResendVerificationEmail(context.Background(), user.ID); err != nil {
		t.Fatalf("ResendVerificationEmail() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox)
	}
}

func TestAuthServiceResendVerificationEmailByAddressForUnverifiedUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	if _, err := service.Register(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(outbox(t, store))

	if err := service.ResendVerificationEmailByAddress(context.Background(), "USER@example.com"); err != nil {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox+1 {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox+1)
	}
}

func TestAuthServiceResendVerificationEmailByAddressNoOpForMissingUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store
	initialOutbox := len(outbox(t, store))

	if err := service.ResendVerificationEmailByAddress(context.Background(), "missing@example.com"); err != nil {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox)
	}
}

func TestAuthServiceResendVerificationEmailByAddressNoOpForVerifiedUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	markUserVerified(t, store, user.ID)
	initialOutbox := len(outbox(t, store))

	if err := service.ResendVerificationEmailByAddress(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox)
	}
}

func TestAuthServiceResendVerificationEmailByAddressRejectsInvalidEmail(t *testing.T) {
	service := newTestAuthService(t)

	err := service.ResendVerificationEmailByAddress(context.Background(), "not-an-email")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("ResendVerificationEmailByAddress() error = %v, want %v", err, ErrInvalidEmail)
	}
}

func TestAuthServiceRequestPasswordReset(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store

	if _, err := service.Register(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(outbox(t, store))

	if err := service.RequestPasswordReset(context.Background(), "USER@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox+1 {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox+1)
	}
	if countRows(t, store, "password_reset_tokens") != 1 {
		t.Fatalf("password reset token count = %d, want 1", countRows(t, store, "password_reset_tokens"))
	}
}

func TestAuthServiceRequestPasswordResetNoOpForMissingUser(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store
	initialOutbox := len(outbox(t, store))

	if err := service.RequestPasswordReset(context.Background(), "missing@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if len(outbox(t, store)) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(outbox(t, store)), initialOutbox)
	}
	if countRows(t, store, "password_reset_tokens") != 0 {
		t.Fatalf("password reset token count = %d, want 0", countRows(t, store, "password_reset_tokens"))
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
	store := service.store

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
	store := service.store

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

func mustNewAuthService(t *testing.T, opts AuthOptions) *AuthService {
	t.Helper()
	if opts.TOTPSecretKey == nil {
		opts.TOTPSecretKey = testTOTPSecretKey
	}
	service, err := NewAuthService(sqlitetest.New(t), opts)
	if err != nil {
		t.Fatalf("NewAuthService() error = %v", err)
	}
	return service
}

func newTestAuthService(t *testing.T) *AuthService {
	t.Helper()

	return mustNewAuthService(t, AuthOptions{
		SessionDuration:     time.Hour,
		PasswordMinLen:      8,
		Argon2idMemoryKiB:   64,
		Argon2idIterations:  1,
		Argon2idParallelism: 1,
		TOTPBackupCodeKey:   []byte("test-backup-code-key"),
		EmailOptions: email.MessageOptions{
			AppBaseURL: "http://localhost:8080",
			From:       "Go Spark <hello@example.com>",
		},
	})
}

func newTestAuthServiceWithNoticeDisabled(t *testing.T) *AuthService {
	t.Helper()

	return mustNewAuthService(t, AuthOptions{
		SessionDuration:     time.Hour,
		PasswordMinLen:      8,
		Argon2idMemoryKiB:   64,
		Argon2idIterations:  1,
		Argon2idParallelism: 1,
		TOTPBackupCodeKey:   []byte("test-backup-code-key"),
		EmailOptions: email.MessageOptions{
			AppBaseURL: "http://localhost:8080",
			From:       "Go Spark <hello@example.com>",
		},
		EmailChangeNoticeEnabled: boolPtr(false),
	})
}

func boolPtr(v bool) *bool {
	return &v
}

// outbox reads the emails the service has enqueued, in insertion order. It
// replaces the in-memory fake's outbox slice now that the service writes to the
// real email_outbox table.
func outbox(t *testing.T, s *authStore) []email.Message {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT sender, recipient, subject, text_body, html_body FROM email_outbox ORDER BY id`)
	if err != nil {
		t.Fatalf("query email_outbox: %v", err)
	}
	defer rows.Close()

	var msgs []email.Message
	for rows.Next() {
		var m email.Message
		if err := rows.Scan(&m.From, &m.To, &m.Subject, &m.TextBody, &m.HTMLBody); err != nil {
			t.Fatalf("scan email_outbox: %v", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate email_outbox: %v", err)
	}
	return msgs
}

// countRows returns the number of rows in a table, replacing the in-memory
// fake's map-length checks. Test-only: it interpolates the table name into the
// query, which is safe here because every caller passes a hardcoded literal —
// do not lift this into non-test code.
func countRows(t *testing.T, s *authStore, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// sessionExists reports whether a session row with the given token hash exists.
func sessionExists(t *testing.T, s *authStore, tokenHash string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM sessions WHERE token_hash = ?", tokenHash).Scan(&n); err != nil {
		t.Fatalf("query session: %v", err)
	}
	return n > 0
}

// sessionID returns the id of the session row with the given token hash.
func sessionID(t *testing.T, s *authStore, tokenHash string) int64 {
	t.Helper()
	var id int64
	if err := s.db.QueryRowContext(context.Background(),
		"SELECT id FROM sessions WHERE token_hash = ?", tokenHash).Scan(&id); err != nil {
		t.Fatalf("query session id: %v", err)
	}
	return id
}

// seedEmailChangeToken inserts an email-change token directly, standing in for
// the fake's map writes when a test needs a token in a specific state.
func seedEmailChangeToken(t *testing.T, s *authStore, userID int64, newEmail, tokenHash string, expiresAt time.Time) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO email_change_tokens (user_id, new_email, token_hash, expires_at)
		 VALUES (?, ?, ?, ?)`, userID, newEmail, tokenHash, expiresAt); err != nil {
		t.Fatalf("seed email change token: %v", err)
	}
}

// singleEmailChangeToken returns the only email-change token, failing if there
// is not exactly one.
func singleEmailChangeToken(t *testing.T, s *authStore) db.EmailChangeToken {
	t.Helper()
	var tok db.EmailChangeToken
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT id, user_id, new_email, token_hash, expires_at, consumed_at, created_at
		 FROM email_change_tokens`).Scan(
		&tok.ID, &tok.UserID, &tok.NewEmail, &tok.TokenHash, &tok.ExpiresAt, &tok.ConsumedAt, &tok.CreatedAt); err != nil {
		t.Fatalf("query email change token: %v", err)
	}
	return tok
}

// markUserVerified flips a user's email_verified_at, standing in for the fake's
// direct map mutation.
func markUserVerified(t *testing.T, s *authStore, userID int64) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE users SET email_verified_at = ? WHERE id = ?`,
		time.Now().UTC(), userID); err != nil {
		t.Fatalf("mark user verified: %v", err)
	}
}
