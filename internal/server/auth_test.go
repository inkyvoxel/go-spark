package server

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
)

func TestLoadSessionAddsCurrentUserToContext(t *testing.T) {
	wantUser := db.User{ID: 42, Email: "user@example.com"}
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
	req = req.WithContext(contextWithUser(req.Context(), db.User{ID: 42, Email: "user@example.com"}))
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
	if location := rec.Header().Get("Location"); location != "/login?next=%2Fprivate%3Ftab%3Done" {
		t.Fatalf("Location = %q, want %q", location, "/login?next=%2Fprivate%3Ftab%3Done")
	}
}

func TestRequireAnonymousRedirectsCurrentUserToAccount(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req = req.WithContext(contextWithUser(req.Context(), db.User{
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
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want %q", location, "/account")
	}
}

func TestRequireAnonymousRedirectsUnverifiedUserToVerifyEmail(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req = req.WithContext(contextWithUser(req.Context(), db.User{ID: 42, Email: "user@example.com"}))
	rec := httptest.NewRecorder()

	srv.requireAnonymous(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != verifyEmailPath {
		t.Fatalf("Location = %q, want %q", location, verifyEmailPath)
	}
}

func TestRequireAnonymousAllowsAnonymousUser(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	srv.requireAnonymous(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireVerifiedAuthAllowsVerifiedUser(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodPost, "/account/change-password", nil)
	req = req.WithContext(contextWithUser(req.Context(), db.User{
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

	req := httptest.NewRequest(http.MethodPost, "/account/change-password", nil)
	req = req.WithContext(contextWithUser(req.Context(), db.User{
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

func TestRequireVerifiedAuthRedirectsUnverifiedGETToVerifyPage(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req = req.WithContext(contextWithUser(req.Context(), db.User{
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
	if location := rec.Header().Get("Location"); location != verifyEmailPath {
		t.Fatalf("Location = %q, want %q", location, verifyEmailPath)
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
		{name: "path", value: "/account", want: "/account"},
		{name: "path with query", value: "/account?tab=profile", want: "/account?tab=profile"},
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
	user                 db.User
	token                string
	err                  error
	registered           bool
	registerEmail        string
	registerPass         string
	registerErr          error
	loginEmail           string
	loginPass            string
	loginSession         db.Session
	loginErr             error
	logoutToken          string
	logoutErr            error
	verifyToken          string
	verifyErr            error
	publicResendEmail    string
	publicResendErr      error
	publicResendCalls    int
	resendUser           db.User
	resendErr            error
	resendCalled         bool
	changePasswordUser   db.User
	changePasswordOld    string
	changePasswordNew    string
	changePasswordErr    error
	changePasswordCalled bool
	requestResetEmail    string
	requestResetErr      error
	validateResetToken   string
	validateResetErr     error
	resetToken           string
	resetNewPassword     string
	resetErr             error
}

func (f *fakeAuthLookup) UserBySessionToken(ctx context.Context, token string) (db.User, error) {
	f.token = token
	return f.user, f.err
}

func (f *fakeAuthLookup) Login(ctx context.Context, email string, password string) (db.User, db.Session, error) {
	f.loginEmail = email
	f.loginPass = password
	if f.loginErr != nil {
		return db.User{}, db.Session{}, f.loginErr
	}
	return f.user, f.loginSession, nil
}

func (f *fakeAuthLookup) Logout(ctx context.Context, token string) error {
	f.logoutToken = token
	return f.logoutErr
}

func (f *fakeAuthLookup) Register(ctx context.Context, email string, password string) (db.User, error) {
	f.registered = true
	f.registerEmail = email
	f.registerPass = password
	return f.user, f.registerErr
}

func (f *fakeAuthLookup) VerifyEmail(ctx context.Context, token string) (db.User, error) {
	f.verifyToken = token
	if f.verifyErr != nil {
		return db.User{}, f.verifyErr
	}
	return f.user, nil
}

func (f *fakeAuthLookup) ResendVerificationEmailByAddress(ctx context.Context, email string) error {
	f.publicResendCalls++
	f.publicResendEmail = email
	return f.publicResendErr
}

func (f *fakeAuthLookup) ResendVerificationEmail(ctx context.Context, user db.User) error {
	f.resendCalled = true
	f.resendUser = user
	return f.resendErr
}

func (f *fakeAuthLookup) ChangePassword(ctx context.Context, user db.User, currentPassword, newPassword string) error {
	f.changePasswordCalled = true
	f.changePasswordUser = user
	f.changePasswordOld = currentPassword
	f.changePasswordNew = newPassword
	return f.changePasswordErr
}

func (f *fakeAuthLookup) RequestPasswordReset(ctx context.Context, email string) error {
	f.requestResetEmail = email
	return f.requestResetErr
}

func (f *fakeAuthLookup) ValidatePasswordResetToken(ctx context.Context, token string) error {
	f.validateResetToken = token
	return f.validateResetErr
}

func (f *fakeAuthLookup) ResetPasswordWithToken(ctx context.Context, token, newPassword string) error {
	f.resetToken = token
	f.resetNewPassword = newPassword
	return f.resetErr
}

func newAuthMiddlewareTestServer(auth authService) *Server {
	return &Server{
		auth:   auth,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}
