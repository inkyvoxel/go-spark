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
	"github.com/inkyvoxel/go-spark/internal/paths"
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
	store := service.store.(*fakeAuthStore)
	storedUser := store.usersByID[user.ID]
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

	if len(store.verificationTokens) != 1 {
		t.Fatalf("verification token count = %d, want 1", len(store.verificationTokens))
	}
	if len(store.outbox) != 1 {
		t.Fatalf("outbox count = %d, want 1", len(store.outbox))
	}
	if store.outbox[0].To != "<user@example.com>" {
		t.Fatalf("confirmation email recipient = %q, want <user@example.com>", store.outbox[0].To)
	}
	if !strings.Contains(store.outbox[0].TextBody, "http://localhost:8080"+paths.ConfirmEmail+"?token=") {
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
	if len(session.Token) != 64 {
		t.Fatalf("session token length = %d, want %d", len(session.Token), 64)
	}
	if time.Until(session.ExpiresAt) <= 0 {
		t.Fatalf("session ExpiresAt = %s, want future time", session.ExpiresAt)
	}
	store := service.store.(*fakeAuthStore)
	if _, ok := store.sessions[session.Token]; ok {
		t.Fatal("raw session token stored in fake auth store, want hashed-only storage")
	}
	if _, ok := store.sessions[hashToken(session.Token)]; !ok {
		t.Fatal("session hash not found in fake auth store")
	}
}

func TestAuthServiceWithPepperSupportsRegisterLoginAndPasswordChange(t *testing.T) {
	service := NewAuthService(newFakeAuthStore(), AuthOptions{
		SessionDuration:     time.Hour,
		PasswordMinLen:      8,
		Argon2idMemoryKiB:   64,
		Argon2idIterations:  1,
		Argon2idParallelism: 1,
		Argon2idSaltLength:  16,
		Argon2idKeyLength:   32,
		PasswordPepper:      "test-pepper",
		ConfirmationEmail: email.AccountConfirmationOptions{
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
	store := service.store.(*fakeAuthStore)

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
	currentStoreSession := store.sessions[hashToken(currentSession.Token)]
	otherStoreSession := store.sessions[hashToken(otherSession.Token)]

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

	if err := service.RevokeSessionByID(context.Background(), user.ID, currentSession.Token, currentStoreSession.ID); !errors.Is(err, ErrCannotRevokeCurrentSession) {
		t.Fatalf("RevokeSessionByID(current) error = %v, want %v", err, ErrCannotRevokeCurrentSession)
	}

	if err := service.RevokeSessionByID(context.Background(), user.ID, currentSession.Token, otherStoreSession.ID); err != nil {
		t.Fatalf("RevokeSessionByID(other) error = %v", err)
	}
	if _, ok := store.sessions[hashToken(otherSession.Token)]; ok {
		t.Fatal("other session still present after revoke")
	}

	_, anotherSession, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := service.RevokeOtherSessions(context.Background(), user.ID, currentSession.Token); err != nil {
		t.Fatalf("RevokeOtherSessions() error = %v", err)
	}
	if _, ok := store.sessions[hashToken(currentSession.Token)]; !ok {
		t.Fatal("current session missing after revoke others")
	}
	if _, ok := store.sessions[hashToken(anotherSession.Token)]; ok {
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

func TestAuthServiceChangePasswordWrapsStoreErrors(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	store.updateUserPasswordHashErr = errors.New("database unavailable")
	err = service.ChangePassword(context.Background(), user.ID, "password", "new-password")
	if err == nil {
		t.Fatal("ChangePassword() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "set password and revoke sessions") {
		t.Fatalf("ChangePassword() error = %v, want operation context", err)
	}

	store.updateUserPasswordHashErr = nil
	store.deleteSessionsByUserIDErr = errors.New("database unavailable")
	err = service.ChangePassword(context.Background(), user.ID, "password", "new-password")
	if err == nil {
		t.Fatal("ChangePassword() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "set password and revoke sessions") {
		t.Fatalf("ChangePassword() error = %v, want operation context", err)
	}
}

func TestAuthServiceRequestEmailChange(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := service.RequestEmailChange(context.Background(), user.ID, "password", "NEW@example.com"); err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}

	if len(store.emailChangeTokens) != 1 {
		t.Fatalf("email change token count = %d, want 1", len(store.emailChangeTokens))
	}
	var token db.EmailChangeToken
	for _, item := range store.emailChangeTokens {
		token = item
	}
	if token.UserID != user.ID {
		t.Fatalf("email change token user ID = %d, want %d", token.UserID, user.ID)
	}
	if token.NewEmail != "new@example.com" {
		t.Fatalf("email change token new email = %q, want normalized email", token.NewEmail)
	}
	if len(store.outbox) != 2 {
		t.Fatalf("outbox count = %d, want registration and email-change messages", len(store.outbox))
	}
	message := store.outbox[len(store.outbox)-1]
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
		{name: "already registered", password: "password", email: "taken@example.com", want: ErrEmailAlreadyRegistered},
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

func TestAuthServiceRequestEmailChangeWrapsStoreErrors(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	store.emailChangeRequestErr = errors.New("database unavailable")
	err = service.RequestEmailChange(context.Background(), user.ID, "password", "new@example.com")
	if err == nil {
		t.Fatal("RequestEmailChange() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "request email change") {
		t.Fatalf("RequestEmailChange() error = %v, want operation context", err)
	}
}

func TestAuthServiceRequestEmailChangeOptionalModeAppliesImmediately(t *testing.T) {
	service := newTestAuthServiceWithEmailVerificationRequired(t, false)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	_, session, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	initialOutbox := len(store.outbox)

	if err := service.RequestEmailChange(context.Background(), user.ID, "password", "new@example.com"); err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}

	if len(store.emailChangeTokens) != 0 {
		t.Fatalf("email change token count = %d, want 0", len(store.emailChangeTokens))
	}
	changed, err := store.GetUserByEmail(context.Background(), "new@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail(new) error = %v", err)
	}
	if changed.Email != "new@example.com" {
		t.Fatalf("changed email = %q, want %q", changed.Email, "new@example.com")
	}
	if !changed.EmailVerifiedAt.Valid {
		t.Fatal("changed EmailVerifiedAt.Valid = false, want true")
	}
	if _, err := store.GetUserByEmail(context.Background(), "user@example.com"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetUserByEmail(old) error = %v, want %v", err, sql.ErrNoRows)
	}
	if _, err := service.UserBySessionToken(context.Background(), session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old session error = %v, want %v", err, ErrInvalidSession)
	}
	if len(store.outbox) != initialOutbox+1 {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox+1)
	}
	notice := store.outbox[len(store.outbox)-1]
	if notice.To != "<user@example.com>" {
		t.Fatalf("old email notice To = %q, want old email", notice.To)
	}
}

func TestAuthServiceRequestEmailChangeOptionalModeWrapsStoreErrors(t *testing.T) {
	service := newTestAuthServiceWithEmailVerificationRequired(t, false)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	store.changeEmailImmediatelyErr = errors.New("database unavailable")
	err = service.RequestEmailChange(context.Background(), user.ID, "password", "new@example.com")
	if err == nil {
		t.Fatal("RequestEmailChange() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "change email immediately") {
		t.Fatalf("RequestEmailChange() error = %v, want operation context", err)
	}
}

func TestAuthServiceRequestEmailChangeOptionalModeSkipsOldEmailNoticeWhenDisabled(t *testing.T) {
	service := newTestAuthServiceWithEmailVerificationAndNotice(t, false, false)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(store.outbox)

	if err := service.RequestEmailChange(context.Background(), user.ID, "password", "new@example.com"); err != nil {
		t.Fatalf("RequestEmailChange() error = %v", err)
	}

	if len(store.outbox) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox)
	}
}

func TestAuthServiceConfirmEmailChange(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	_, session, err := service.Login(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	store.emailChangeTokens[hashToken("raw-token")] = db.EmailChangeToken{
		ID:        1,
		UserID:    user.ID,
		NewEmail:  "new@example.com",
		TokenHash: hashToken("raw-token"),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		CreatedAt: time.Now().UTC(),
	}

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
	notice := store.outbox[len(store.outbox)-1]
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
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store.emailChangeTokens[hashToken("raw-token")] = db.EmailChangeToken{
		ID:        1,
		UserID:    user.ID,
		NewEmail:  "new@example.com",
		TokenHash: hashToken("raw-token"),
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
		CreatedAt: time.Now().UTC(),
	}

	_, err = service.ConfirmEmailChange(context.Background(), "raw-token")
	if !errors.Is(err, ErrInvalidEmailChangeToken) {
		t.Fatalf("ConfirmEmailChange() error = %v, want %v", err, ErrInvalidEmailChangeToken)
	}
}

func TestAuthServiceConfirmEmailChangeRejectsAlreadyOwnedEmail(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := service.Register(context.Background(), "new@example.com", "password"); err != nil {
		t.Fatalf("Register() competing email error = %v", err)
	}
	store.emailChangeTokens[hashToken("raw-token")] = db.EmailChangeToken{
		ID:        1,
		UserID:    user.ID,
		NewEmail:  "new@example.com",
		TokenHash: hashToken("raw-token"),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		CreatedAt: time.Now().UTC(),
	}

	_, err = service.ConfirmEmailChange(context.Background(), "raw-token")
	if !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("ConfirmEmailChange() error = %v, want %v", err, ErrEmailAlreadyRegistered)
	}
}

func TestAuthServiceConfirmEmailChangeSkipsOldEmailNoticeWhenDisabled(t *testing.T) {
	service := newTestAuthServiceWithEmailVerificationAndNotice(t, true, false)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	initialOutbox := len(store.outbox)
	store.emailChangeTokens[hashToken("raw-token")] = db.EmailChangeToken{
		ID:        1,
		UserID:    user.ID,
		NewEmail:  "new@example.com",
		TokenHash: hashToken("raw-token"),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		CreatedAt: time.Now().UTC(),
	}

	if _, err := service.ConfirmEmailChange(context.Background(), "raw-token"); err != nil {
		t.Fatalf("ConfirmEmailChange() error = %v", err)
	}
	if len(store.outbox) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox)
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

	if err := service.ResendVerificationEmail(context.Background(), user.ID); err != nil {
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
	storedUser := store.usersByID[user.ID]
	storedUser.EmailVerifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	store.usersByID[user.ID] = storedUser
	store.usersByEmail[user.Email] = storedUser
	initialOutbox := len(store.outbox)

	if err := service.ResendVerificationEmail(context.Background(), user.ID); err != nil {
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

	err = service.ResendVerificationEmail(context.Background(), user.ID)
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
	storedUser := store.usersByID[user.ID]
	storedUser.EmailVerifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	store.usersByID[user.ID] = storedUser
	store.usersByEmail[user.Email] = storedUser
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

func TestAuthServiceRegisterOptionalVerificationCreatesVerifiedUserWithoutOutbox(t *testing.T) {
	service := newTestAuthServiceWithEmailVerificationRequired(t, false)
	store := service.store.(*fakeAuthStore)

	user, err := service.Register(context.Background(), "user@example.com", "password")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !user.EmailVerifiedAt.Valid {
		t.Fatal("EmailVerifiedAt.Valid = false, want true")
	}
	if len(store.verificationTokens) != 0 {
		t.Fatalf("verification token count = %d, want 0", len(store.verificationTokens))
	}
	if len(store.outbox) != 0 {
		t.Fatalf("outbox count = %d, want 0", len(store.outbox))
	}
}

func TestAuthServiceResendVerificationEmailOptionalModeNoOp(t *testing.T) {
	service := newTestAuthServiceWithEmailVerificationRequired(t, false)
	store := service.store.(*fakeAuthStore)
	user, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	initialOutbox := len(store.outbox)
	if err := service.ResendVerificationEmail(context.Background(), user.ID); err != nil {
		t.Fatalf("ResendVerificationEmail() error = %v", err)
	}
	if len(store.outbox) != initialOutbox {
		t.Fatalf("outbox count = %d, want %d", len(store.outbox), initialOutbox)
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

func newTestAuthServiceWithEmailVerificationRequired(t *testing.T, required bool) *AuthService {
	return newTestAuthServiceWithEmailVerificationAndNotice(t, required, true)
}

func newTestAuthServiceWithEmailVerificationAndNotice(t *testing.T, required bool, noticeEnabled bool) *AuthService {
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
		EmailVerificationPolicy:  NewEmailVerificationPolicy(required),
		EmailChangeNoticeEnabled: boolPtr(noticeEnabled),
	})
}

func boolPtr(v bool) *bool {
	return &v
}

type fakeAuthStore struct {
	nextUserID                int64
	nextSessionID             int64
	nextVerificationTokenID   int64
	nextPasswordResetTokenID  int64
	nextEmailChangeTokenID    int64
	usersByEmail              map[string]db.User
	usersByID                 map[int64]db.User
	sessions                  map[string]db.Session
	verificationTokens        map[string]db.EmailVerificationToken
	passwordResetTokens       map[string]db.PasswordResetToken
	emailChangeTokens         map[string]db.EmailChangeToken
	outbox                    []email.Message
	getUserByEmailErr         error
	resendErr                 error
	updateUserPasswordHashErr error
	deleteSessionsByUserIDErr error
	passwordResetRequestErr   error
	emailChangeRequestErr     error
	changeEmailImmediatelyErr error
	confirmEmailChangeErr     error
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		nextUserID:               1,
		nextSessionID:            1,
		nextVerificationTokenID:  1,
		nextPasswordResetTokenID: 1,
		nextEmailChangeTokenID:   1,
		usersByEmail:             make(map[string]db.User),
		usersByID:                make(map[int64]db.User),
		sessions:                 make(map[string]db.Session),
		verificationTokens:       make(map[string]db.EmailVerificationToken),
		passwordResetTokens:      make(map[string]db.PasswordResetToken),
		emailChangeTokens:        make(map[string]db.EmailChangeToken),
	}
}

func (s *fakeAuthStore) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	if _, ok := s.usersByEmail[email]; ok {
		return User{}, ErrEmailAlreadyRegistered
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

	return userFromDB(user), nil
}

func (s *fakeAuthStore) CreateUserWithEmailVerification(ctx context.Context, params CreateUserWithEmailVerificationParams) (User, error) {
	user, err := s.CreateUser(ctx, params.Email, params.PasswordHash)
	if err != nil {
		return User{}, err
	}
	if _, err := s.CreateEmailVerificationToken(ctx, user.ID, params.TokenHash, params.TokenExpiresAt); err != nil {
		delete(s.usersByEmail, user.Email)
		delete(s.usersByID, user.ID)
		return User{}, err
	}
	s.outbox = append(s.outbox, params.ConfirmationEmail)

	return user, nil
}

func (s *fakeAuthStore) CreateVerifiedUser(ctx context.Context, email, passwordHash string, verifiedAt time.Time) (User, error) {
	user, err := s.CreateUser(ctx, email, passwordHash)
	if err != nil {
		return User{}, err
	}
	stored := s.usersByID[user.ID]
	stored.EmailVerifiedAt = sql.NullTime{Time: verifiedAt, Valid: true}
	s.usersByID[stored.ID] = stored
	s.usersByEmail[stored.Email] = stored
	user.EmailVerifiedAt = stored.EmailVerifiedAt
	return user, nil
}

func (s *fakeAuthStore) GetUserByEmail(ctx context.Context, email string) (UserRecord, error) {
	if s.getUserByEmailErr != nil {
		return UserRecord{}, s.getUserByEmailErr
	}
	user, ok := s.usersByEmail[email]
	if !ok {
		return UserRecord{}, sql.ErrNoRows
	}

	return userRecordFromDB(user), nil
}

func (s *fakeAuthStore) GetUserByID(ctx context.Context, userID int64) (UserRecord, error) {
	user, ok := s.usersByID[userID]
	if !ok {
		return UserRecord{}, sql.ErrNoRows
	}
	return userRecordFromDB(user), nil
}

func (s *fakeAuthStore) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (SessionRecord, error) {
	session := db.Session{
		ID:        s.nextSessionID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	s.nextSessionID++
	s.sessions[tokenHash] = session

	return sessionRecordFromDB(session), nil
}

func (s *fakeAuthStore) GetUserBySessionTokenHash(ctx context.Context, tokenHash string) (UserRecord, error) {
	session, ok := s.sessions[tokenHash]
	if !ok || !session.ExpiresAt.After(time.Now().UTC()) {
		return UserRecord{}, sql.ErrNoRows
	}

	user, ok := s.usersByID[session.UserID]
	if !ok {
		return UserRecord{}, sql.ErrNoRows
	}

	return userRecordFromDB(user), nil
}

func (s *fakeAuthStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	delete(s.sessions, tokenHash)
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

func (s *fakeAuthStore) ListActiveSessionsByUserID(ctx context.Context, userID int64) ([]SessionRecord, error) {
	sessions := make([]SessionRecord, 0, len(s.sessions))
	now := time.Now().UTC()
	for _, session := range s.sessions {
		if session.UserID == userID && session.ExpiresAt.After(now) {
			sessions = append(sessions, sessionRecordFromDB(session))
		}
	}
	return sessions, nil
}

func (s *fakeAuthStore) DeleteOtherSessionsByUserIDAndTokenHash(ctx context.Context, userID int64, tokenHash string) (int64, error) {
	var deleted int64
	for token, session := range s.sessions {
		if session.UserID == userID && token != tokenHash {
			delete(s.sessions, token)
			deleted++
		}
	}
	return deleted, nil
}

func (s *fakeAuthStore) DeleteSessionByIDAndUserIDAndTokenHashNot(ctx context.Context, sessionID, userID int64, tokenHash string) (int64, error) {
	for token, session := range s.sessions {
		if session.ID == sessionID && session.UserID == userID && token != tokenHash {
			delete(s.sessions, token)
			return 1, nil
		}
	}
	return 0, nil
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

func (s *fakeAuthStore) SetPasswordAndRevokeSessions(ctx context.Context, userID int64, passwordHash string) error {
	if s.updateUserPasswordHashErr != nil {
		return s.updateUserPasswordHashErr
	}
	if s.deleteSessionsByUserIDErr != nil {
		return s.deleteSessionsByUserIDErr
	}

	user, ok := s.usersByID[userID]
	if !ok {
		return sql.ErrNoRows
	}

	user.PasswordHash = passwordHash
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user

	for token, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, token)
		}
	}

	return nil
}

func (s *fakeAuthStore) CreateEmailVerificationToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (EmailVerificationToken, error) {
	token := db.EmailVerificationToken{
		ID:        s.nextVerificationTokenID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	s.nextVerificationTokenID++
	s.verificationTokens[tokenHash] = token

	return emailVerificationTokenFromDB(token), nil
}

func (s *fakeAuthStore) CreatePasswordResetToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (PasswordResetToken, error) {
	token := db.PasswordResetToken{
		ID:        s.nextPasswordResetTokenID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	s.nextPasswordResetTokenID++
	s.passwordResetTokens[tokenHash] = token

	return passwordResetTokenFromDB(token), nil
}

func (s *fakeAuthStore) GetValidPasswordResetTokenByHash(ctx context.Context, tokenHash string, now time.Time) (PasswordResetToken, error) {
	token, ok := s.passwordResetTokens[tokenHash]
	if !ok || token.ConsumedAt.Valid || !token.ExpiresAt.After(now) {
		return PasswordResetToken{}, sql.ErrNoRows
	}

	return passwordResetTokenFromDB(token), nil
}

func (s *fakeAuthStore) ConsumePasswordResetToken(ctx context.Context, tokenHash string, consumedAt time.Time) (PasswordResetToken, error) {
	token, ok := s.passwordResetTokens[tokenHash]
	if !ok || token.ConsumedAt.Valid || !token.ExpiresAt.After(consumedAt) {
		return PasswordResetToken{}, sql.ErrNoRows
	}
	token.ConsumedAt = sql.NullTime{Time: consumedAt, Valid: true}
	s.passwordResetTokens[tokenHash] = token

	return passwordResetTokenFromDB(token), nil
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

func (s *fakeAuthStore) RequestEmailChange(ctx context.Context, params RequestEmailChangeParams) error {
	if s.emailChangeRequestErr != nil {
		return s.emailChangeRequestErr
	}
	token := db.EmailChangeToken{
		ID:        s.nextEmailChangeTokenID,
		UserID:    params.UserID,
		NewEmail:  params.NewEmail,
		TokenHash: params.TokenHash,
		ExpiresAt: params.TokenExpiresAt,
		CreatedAt: time.Now().UTC(),
	}
	s.nextEmailChangeTokenID++
	s.emailChangeTokens[params.TokenHash] = token
	s.outbox = append(s.outbox, params.EmailChangeVerifyEmail)
	return nil
}

func (s *fakeAuthStore) ChangeEmailImmediately(ctx context.Context, params ChangeEmailImmediatelyParams) (User, error) {
	if s.changeEmailImmediatelyErr != nil {
		return User{}, s.changeEmailImmediatelyErr
	}
	existing, ok := s.usersByEmail[params.NewEmail]
	if ok && existing.ID != params.UserID {
		return User{}, ErrEmailAlreadyRegistered
	}

	user, ok := s.usersByID[params.UserID]
	if !ok {
		return User{}, sql.ErrNoRows
	}

	delete(s.usersByEmail, user.Email)
	oldEmail := user.Email
	user.Email = params.NewEmail
	user.EmailVerifiedAt = sql.NullTime{Time: params.ChangedAt, Valid: true}
	s.usersByID[user.ID] = user
	s.usersByEmail[user.Email] = user

	if err := s.DeleteSessionsByUserID(ctx, user.ID); err != nil {
		return User{}, err
	}
	if params.SendOldEmailNotice {
		notice, err := email.NewEmailChangeNoticeMessage(params.OldEmailNoticeOptions, oldEmail)
		if err != nil {
			return User{}, err
		}
		s.outbox = append(s.outbox, notice)
	}
	return userFromDB(user), nil
}

func (s *fakeAuthStore) ConfirmEmailChange(ctx context.Context, params ConfirmEmailChangeParams) (User, error) {
	if s.confirmEmailChangeErr != nil {
		return User{}, s.confirmEmailChangeErr
	}
	token, ok := s.emailChangeTokens[params.TokenHash]
	if !ok || token.ConsumedAt.Valid || !token.ExpiresAt.After(params.ChangedAt) {
		return User{}, sql.ErrNoRows
	}
	if existing, ok := s.usersByEmail[token.NewEmail]; ok && existing.ID != token.UserID {
		return User{}, ErrEmailAlreadyRegistered
	}

	user, ok := s.usersByID[token.UserID]
	if !ok {
		return User{}, sql.ErrNoRows
	}

	delete(s.usersByEmail, user.Email)
	oldEmail := user.Email
	user.Email = token.NewEmail
	user.EmailVerifiedAt = sql.NullTime{Time: params.ChangedAt, Valid: true}
	s.usersByID[user.ID] = user
	s.usersByEmail[user.Email] = user

	token.ConsumedAt = sql.NullTime{Time: params.ChangedAt, Valid: true}
	s.emailChangeTokens[params.TokenHash] = token

	if err := s.DeleteSessionsByUserID(ctx, user.ID); err != nil {
		return User{}, err
	}
	if params.SendOldEmailNotice {
		notice, err := email.NewEmailChangeNoticeMessage(params.OldEmailNoticeOptions, oldEmail)
		if err != nil {
			return User{}, err
		}
		s.outbox = append(s.outbox, notice)
	}

	return userFromDB(user), nil
}

func (s *fakeAuthStore) VerifyEmailByTokenHash(ctx context.Context, tokenHash string, verifiedAt time.Time) (User, error) {
	token, ok := s.verificationTokens[tokenHash]
	if !ok || token.ConsumedAt.Valid || !token.ExpiresAt.After(verifiedAt) {
		return User{}, sql.ErrNoRows
	}

	user, ok := s.usersByID[token.UserID]
	if !ok {
		return User{}, sql.ErrNoRows
	}

	token.ConsumedAt = sql.NullTime{Time: verifiedAt, Valid: true}
	s.verificationTokens[tokenHash] = token

	user.EmailVerifiedAt = sql.NullTime{Time: verifiedAt, Valid: true}
	s.usersByID[user.ID] = user
	s.usersByEmail[user.Email] = user

	return userFromDB(user), nil
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

func userFromDB(row db.User) User {
	return User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		CreatedAt:       row.CreatedAt,
	}
}

func userRecordFromDB(row db.User) UserRecord {
	return UserRecord{
		User:         userFromDB(row),
		PasswordHash: row.PasswordHash,
	}
}

func sessionRecordFromDB(row db.Session) SessionRecord {
	return SessionRecord{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}
}

func emailVerificationTokenFromDB(row db.EmailVerificationToken) EmailVerificationToken {
	return EmailVerificationToken{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func passwordResetTokenFromDB(row db.PasswordResetToken) PasswordResetToken {
	return PasswordResetToken{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}
