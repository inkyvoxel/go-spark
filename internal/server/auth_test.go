package server

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

func TestLoadSessionAddsCurrentUserToContext(t *testing.T) {
	wantUser := services.User{ID: 42, Email: "user@example.com"}
	auth := &fakeAuthLookup{
		user: wantUser,
	}
	srv := newAuthMiddlewareTestServer(auth)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r.Context())
		if !ok {
			t.Fatal("currentUser() ok = false, want true")
		}
		if user.ID != wantUser.ID {
			t.Fatalf("current user ID = %d, want %d", user.ID, wantUser.ID)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.loadSession(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if auth.token != "session-token" {
		t.Fatalf("auth lookup token = %q, want %q", auth.token, "session-token")
	}
}

func TestLoadSessionAllowsMissingCookie(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()

	srv.loadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(r.Context()); ok {
			t.Fatal("currentUser() ok = true, want false")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestLoadSessionAllowsInvalidSessionAsAnonymous(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{
		err: services.ErrInvalidSession,
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bad-token"})
	rec := httptest.NewRecorder()

	srv.loadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(r.Context()); ok {
			t.Fatal("currentUser() ok = true, want false")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.MaxAge != -1 {
		t.Fatalf("session MaxAge = %d, want %d", session.MaxAge, -1)
	}
}

func TestSetSessionCookieSetsPositiveMaxAge(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()

	srv.setSessionCookie(rec, req, services.AuthSession{
		Token:     "session-token",
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	})

	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.MaxAge <= 0 {
		t.Fatalf("session MaxAge = %d, want positive value", session.MaxAge)
	}
}

func TestLoadSessionHandlesLookupError(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{
		err: errors.New("database unavailable"),
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	srv.loadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRequireAuthAllowsCurrentUser(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{ID: 42, Email: "user@example.com"}))
	rec := httptest.NewRecorder()

	srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireAuthRejectsMissingCurrentUser(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodPost, "/private", nil)
	rec := httptest.NewRecorder()

	srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthRedirectsAnonymousGETToLogin(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/private?tab=one", nil)
	rec := httptest.NewRecorder()

	srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Login+"?next=%2Fprivate%3Ftab%3Done" {
		t.Fatalf("Location = %q, want %q", location, paths.Login+"?next=%2Fprivate%3Ftab%3Done")
	}
}

func TestRequireAnonymousRedirectsCurrentUserToAccount(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{
		ID:    42,
		Email: "user@example.com",
		EmailVerifiedAt: sql.NullTime{
			Time:  time.Now().UTC(),
			Valid: true,
		},
	}))
	rec := httptest.NewRecorder()

	srv.requireAnonymous(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Account {
		t.Fatalf("Location = %q, want %q", location, paths.Account)
	}
}

func TestRequireAnonymousRedirectsUnverifiedUserToVerifyEmail(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{ID: 42, Email: "user@example.com"}))
	rec := httptest.NewRecorder()

	srv.requireAnonymous(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
	}
}

func TestRequireAnonymousAllowsAnonymousUser(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	rec := httptest.NewRecorder()

	srv.requireAnonymous(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireAnonymousRedirectsUnverifiedUserToAccountWhenVerificationOptional(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})
	srv.emailVerificationPolicy = services.NewEmailVerificationPolicy(false)

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{ID: 42, Email: "user@example.com"}))
	rec := httptest.NewRecorder()

	srv.requireAnonymous(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Account {
		t.Fatalf("Location = %q, want %q", location, paths.Account)
	}
}

func TestRequireVerifiedAuthAllowsVerifiedUser(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{
		ID:    42,
		Email: "user@example.com",
		EmailVerifiedAt: sql.NullTime{
			Time:  time.Now().UTC(),
			Valid: true,
		},
	}))
	rec := httptest.NewRecorder()

	srv.requireVerifiedAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireVerifiedAuthRejectsUnverifiedUser(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{
		ID:    42,
		Email: "user@example.com",
	}))
	rec := httptest.NewRecorder()

	srv.requireVerifiedAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireVerifiedAuthAllowsUnverifiedUserWhenVerificationOptional(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})
	srv.emailVerificationPolicy = services.NewEmailVerificationPolicy(false)

	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{
		ID:    42,
		Email: "user@example.com",
	}))
	rec := httptest.NewRecorder()

	srv.requireVerifiedAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireVerifiedAuthRedirectsUnverifiedGETToVerifyPage(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req = req.WithContext(contextWithUser(req.Context(), services.User{
		ID:    42,
		Email: "user@example.com",
	}))
	rec := httptest.NewRecorder()

	srv.requireVerifiedAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
	}
}

func TestRequireVerifiedAuthRejectsAnonymousPost(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodPost, "/private", nil)
	rec := httptest.NewRecorder()

	srv.requireVerifiedAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "path", value: paths.Account, want: paths.Account},
		{name: "path with query", value: paths.Account + "?tab=profile", want: paths.Account + "?tab=profile"},
		{name: "absolute URL", value: "https://evil.example/account", want: ""},
		{name: "protocol relative URL", value: "//evil.example/account", want: ""},
		{name: "missing leading slash", value: "account", want: ""},
		{name: "backslash prefix", value: `\evil.example`, want: ""},
		{name: "slash backslash prefix", value: `/\evil.example`, want: ""},
		{name: "backslash in path", value: `/account\evil`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeRedirectPath(tt.value); got != tt.want {
				t.Fatalf("safeRedirectPath(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestCurrentUserReturnsFalseWhenMissing(t *testing.T) {
	if _, ok := currentUser(context.Background()); ok {
		t.Fatal("currentUser() ok = true, want false")
	}
}

type fakeAuthLookup struct {
	user                 services.User
	token                string
	err                  error
	registered           bool
	registerEmail        string
	registerPass         string
	registerErr          error
	loginEmail           string
	loginPass            string
	loginSession         services.AuthSession
	loginErr             error
	logoutToken          string
	logoutErr            error
	verifyToken          string
	verifyErr            error
	publicResendEmail    string
	publicResendErr      error
	publicResendCalls    int
	resendUser           services.User
	resendErr            error
	resendCalled         bool
	changePasswordUser   services.User
	changePasswordOld    string
	changePasswordNew    string
	changePasswordErr    error
	changePasswordCalled bool
	changeEmailUser      services.User
	changeEmailPassword  string
	changeEmailNewEmail  string
	changeEmailErr       error
	changeEmailCalled    bool
	confirmEmailToken    string
	confirmEmailErr      error
	requestResetEmail    string
	requestResetErr      error
	validateResetToken   string
	validateResetErr     error
	resetToken           string
	resetNewPassword     string
	resetErr             error
	managedSessions      []services.ManagedSession
	listSessionsErr      error
	revokeSessionID      int64
	revokeSessionErr     error
	revokeOtherErr       error
	revokeOtherCalled    bool
}

func (f *fakeAuthLookup) UserBySessionToken(ctx context.Context, token string) (services.User, error) {
	f.token = token
	return f.user, f.err
}

func (f *fakeAuthLookup) Login(ctx context.Context, email string, password string) (services.User, services.AuthSession, error) {
	f.loginEmail = email
	f.loginPass = password
	if f.loginErr != nil {
		return services.User{}, services.AuthSession{}, f.loginErr
	}
	return f.user, f.loginSession, nil
}

func (f *fakeAuthLookup) Logout(ctx context.Context, token string) error {
	f.logoutToken = token
	return f.logoutErr
}

func (f *fakeAuthLookup) Register(ctx context.Context, email string, password string) (services.User, error) {
	f.registered = true
	f.registerEmail = email
	f.registerPass = password
	return f.user, f.registerErr
}

func (f *fakeAuthLookup) VerifyEmail(ctx context.Context, token string) (services.User, error) {
	f.verifyToken = token
	if f.verifyErr != nil {
		return services.User{}, f.verifyErr
	}
	return f.user, nil
}

func (f *fakeAuthLookup) ResendVerificationEmailByAddress(ctx context.Context, email string) error {
	f.publicResendCalls++
	f.publicResendEmail = email
	return f.publicResendErr
}

func (f *fakeAuthLookup) ResendVerificationEmail(ctx context.Context, userID int64) error {
	f.resendCalled = true
	f.resendUser = services.User{ID: userID}
	return f.resendErr
}

func (f *fakeAuthLookup) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	f.changePasswordCalled = true
	f.changePasswordUser = services.User{ID: userID}
	f.changePasswordOld = currentPassword
	f.changePasswordNew = newPassword
	return f.changePasswordErr
}

func (f *fakeAuthLookup) RequestEmailChange(ctx context.Context, userID int64, currentPassword, newEmail string) error {
	f.changeEmailCalled = true
	f.changeEmailUser = services.User{ID: userID}
	f.changeEmailPassword = currentPassword
	f.changeEmailNewEmail = newEmail
	return f.changeEmailErr
}

func (f *fakeAuthLookup) ConfirmEmailChange(ctx context.Context, token string) (services.User, error) {
	f.confirmEmailToken = token
	return f.user, f.confirmEmailErr
}

func (f *fakeAuthLookup) RequestPasswordReset(ctx context.Context, email string) error {
	f.requestResetEmail = email
	return f.requestResetErr
}

func (f *fakeAuthLookup) ValidatePasswordResetToken(ctx context.Context, token string) error {
	f.validateResetToken = token
	if strings.TrimSpace(token) == "" {
		return services.ErrInvalidPasswordResetToken
	}
	return f.validateResetErr
}

func (f *fakeAuthLookup) ResetPasswordWithToken(ctx context.Context, token, newPassword string) error {
	f.resetToken = token
	f.resetNewPassword = newPassword
	if strings.TrimSpace(token) == "" {
		return services.ErrInvalidPasswordResetToken
	}
	return f.resetErr
}

func (f *fakeAuthLookup) ListManagedSessions(ctx context.Context, userID int64, currentSessionToken string) ([]services.ManagedSession, error) {
	return f.managedSessions, f.listSessionsErr
}

func (f *fakeAuthLookup) RevokeOtherSessions(ctx context.Context, userID int64, currentSessionToken string) error {
	f.revokeOtherCalled = true
	return f.revokeOtherErr
}

func (f *fakeAuthLookup) RevokeSessionByID(ctx context.Context, userID int64, currentSessionToken string, sessionID int64) error {
	f.revokeSessionID = sessionID
	return f.revokeSessionErr
}

func (f *fakeAuthLookup) DeleteAccount(ctx context.Context, userID int64, currentPassword string) error {
	return nil
}

func newAuthMiddlewareTestServer(auth authService) *Server {
	return &Server{
		auth:                    auth,
		emailVerificationPolicy: services.DefaultEmailVerificationPolicy(),
		logger:                  slog.New(slog.NewTextHandler(io.Discard, nil)),
		appBaseOrigin:           "http://localhost:8080",
		csrfSigningKey:          []byte("test-csrf-signing-key"),
	}
}
