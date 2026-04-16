package server

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"

	_ "modernc.org/sqlite"
)

func TestRoutesLogin(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
		loginSession: db.Session{
			Token:     "session-token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		"password":    []string{"password"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want %q", location, "/account")
	}
	if auth.loginEmail != "user@example.com" || auth.loginPass != "password" {
		t.Fatalf("login credentials = %q/%q", auth.loginEmail, auth.loginPass)
	}
	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.Value != "session-token" {
		t.Fatalf("session cookie = %q, want %q", session.Value, "session-token")
	}
	if !session.HttpOnly {
		t.Fatal("session cookie HttpOnly = false, want true")
	}
}

func TestRoutesLoginSetsSecureSessionCookieWhenConfigured(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
		loginSession: db.Session{
			Token:     "session-token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	srv := newAuthRouteTestServer(t, auth)
	srv.cookieSecure = true

	form := url.Values{
		"email":       []string{"user@example.com"},
		"password":    []string{"password"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	session := cookieFromRecorder(t, rec, sessionCookieName)
	if !session.Secure {
		t.Fatal("session cookie Secure = false, want true")
	}
}

func TestRoutesLoginRedirectsToSafeNextPath(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
		loginSession: db.Session{
			Token:     "session-token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		"password":    []string{"password"},
		"next":        []string{"/dashboard?tab=home"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/dashboard?tab=home" {
		t.Fatalf("Location = %q, want %q", location, "/dashboard?tab=home")
	}
}

func TestRoutesLoginRejectsUnsafeNextPath(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
		loginSession: db.Session{
			Token:     "session-token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		"password":    []string{"password"},
		"next":        []string{"https://evil.example"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want %q", location, "/account")
	}
}

func TestRoutesLoginFormIncludesSafeNextPath(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/login?next=%2Faccount", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "/account") {
		t.Fatalf("body = %q, want safe next path", rec.Body.String())
	}
}

func TestRoutesLoginFormOmitsUnsafeNextPath(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/login?next=https%3A%2F%2Fevil.example", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), "evil.example") {
		t.Fatalf("body = %q, want unsafe next path omitted", rec.Body.String())
	}
}

func TestRoutesLoginFormShowsResendVerificationLink(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "/resend-verification") {
		t.Fatalf("body = %q, want resend verification link", rec.Body.String())
	}
}

func TestRoutesLoginRedirectsAuthenticatedUserToAccount(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want %q", location, "/account")
	}
}

func TestRoutesRegister(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "new@example.com"},
		loginSession: db.Session{
			Token:     "new-session-token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":            []string{"new@example.com"},
		"password":         []string{"password"},
		"confirm_password": []string{"password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want %q", location, "/account")
	}
	if !auth.registered {
		t.Fatal("Register() was not called")
	}
	if auth.registerEmail != "new@example.com" || auth.registerPass != "password" {
		t.Fatalf("register credentials = %q/%q", auth.registerEmail, auth.registerPass)
	}
	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.Value != "new-session-token" {
		t.Fatalf("session cookie = %q, want %q", session.Value, "new-session-token")
	}
}

func TestRoutesRegisterRejectsMismatchedPasswordConfirmation(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":            []string{"new@example.com"},
		"password":         []string{"password"},
		"confirm_password": []string{"different"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if auth.registered {
		t.Fatal("Register() was called")
	}
	if !strings.Contains(rec.Body.String(), "Passwords do not match.") {
		t.Fatalf("body = %q, want confirmation error", rec.Body.String())
	}
}

func TestRoutesRegisterValidatesRequiredFields(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if auth.registered {
		t.Fatal("Register() was called")
	}
	body := rec.Body.String()
	for _, want := range []string{"Enter your email address.", "Enter a password.", "Confirm your password."} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
}

func TestRoutesRegisterShowsServiceValidationErrors(t *testing.T) {
	auth := &fakeAuthLookup{
		registerErr: services.ErrEmailAlreadyRegistered,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":            []string{"new@example.com"},
		"password":         []string{"password"},
		"confirm_password": []string{"password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "An account with this email already exists.") {
		t.Fatalf("body = %q, want duplicate email error", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), http.StatusText(http.StatusInternalServerError)) {
		t.Fatalf("body = %q, want validation error instead of internal server error", rec.Body.String())
	}
}

func TestRoutesConfirmEmail(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "new@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/confirm-email?token=raw-token", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if auth.verifyToken != "raw-token" {
		t.Fatalf("verify token = %q, want raw-token", auth.verifyToken)
	}
	if !strings.Contains(rec.Body.String(), "Email confirmed") {
		t.Fatalf("body = %q, want confirmation success", rec.Body.String())
	}
}

func TestRoutesConfirmEmailRejectsInvalidToken(t *testing.T) {
	auth := &fakeAuthLookup{
		verifyErr: services.ErrInvalidVerificationToken,
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/confirm-email?token=bad-token", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if auth.verifyToken != "bad-token" {
		t.Fatalf("verify token = %q, want bad-token", auth.verifyToken)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "invalid or has expired") {
		t.Fatalf("body = %q, want invalid token message", body)
	}
	if !strings.Contains(body, "Sign in") {
		t.Fatalf("body = %q, want sign-in link for anonymous user", body)
	}
	if strings.Contains(body, "Go to your account") {
		t.Fatalf("body = %q, did not want account link for anonymous user", body)
	}
}

func TestRoutesConfirmEmailRejectsInvalidTokenForAuthenticatedUser(t *testing.T) {
	auth := &fakeAuthLookup{
		user:      db.User{ID: 1, Email: "user@example.com"},
		verifyErr: services.ErrInvalidVerificationToken,
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/confirm-email?token=bad-token", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "invalid or has expired") {
		t.Fatalf("body = %q, want invalid token message", body)
	}
	if !strings.Contains(body, "Go to your account") {
		t.Fatalf("body = %q, want account link for signed-in user", body)
	}
	if strings.Contains(body, "Sign in") {
		t.Fatalf("body = %q, did not want sign-in link for signed-in user", body)
	}
}

func TestRoutesConfirmEmailHandlesUnexpectedError(t *testing.T) {
	auth := &fakeAuthLookup{
		verifyErr: errors.New("database unavailable"),
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/confirm-email?token=raw-token", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRoutesLogout(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if auth.logoutToken != "session-token" {
		t.Fatalf("logout token = %q, want %q", auth.logoutToken, "session-token")
	}
	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.MaxAge != -1 {
		t.Fatalf("session MaxAge = %d, want %d", session.MaxAge, -1)
	}
}

func TestRoutesHomeShowsAnonymousNav(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sign in") || !strings.Contains(body, "Create account") {
		t.Fatalf("body = %q, want anonymous nav", body)
	}
	if strings.Contains(body, "Sign out") {
		t.Fatalf("body = %q, want it not to contain signed-in nav", body)
	}
}

func TestRoutesHomeShowsAuthenticatedNav(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Account") || !strings.Contains(body, "Sign out") {
		t.Fatalf("body = %q, want signed-in nav", body)
	}
	if strings.Contains(body, "Create account") {
		t.Fatalf("body = %q, want it not to contain anonymous nav", body)
	}
	if auth.token != "session-token" {
		t.Fatalf("session lookup token = %q, want %q", auth.token, "session-token")
	}
}

func TestRoutesAccountRequiresAuth(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "user@example.com") {
		t.Fatalf("body = %q, want account email", rec.Body.String())
	}
	if auth.token != "session-token" {
		t.Fatalf("session lookup token = %q, want %q", auth.token, "session-token")
	}
}

func TestRoutesAccountShowsResendForUnverifiedUser(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "resend-visible") {
		t.Fatalf("body = %q, want resend control", body)
	}
	if !strings.Contains(body, "check-email-visible") {
		t.Fatalf("body = %q, want unverified check-email message", body)
	}
}

func TestRoutesAccountHidesResendForVerifiedUser(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{
			ID:    1,
			Email: "user@example.com",
			EmailVerifiedAt: sql.NullTime{
				Time:  time.Now().UTC(),
				Valid: true,
			},
		},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if strings.Contains(body, "resend-visible") {
		t.Fatalf("body = %q, did not want resend control", body)
	}
	if strings.Contains(body, "check-email-visible") {
		t.Fatalf("body = %q, did not want unverified check-email message", body)
	}
}

func TestRoutesResendVerification(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, "/account/resend-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account?resend=sent" {
		t.Fatalf("Location = %q, want %q", location, "/account?resend=sent")
	}
	if !auth.resendCalled {
		t.Fatal("ResendVerificationEmail() was not called")
	}
	if auth.resendUser.ID != auth.user.ID {
		t.Fatalf("resend user ID = %d, want %d", auth.resendUser.ID, auth.user.ID)
	}
}

func TestRoutesResendVerificationHandlesError(t *testing.T) {
	auth := &fakeAuthLookup{
		user:      db.User{ID: 1, Email: "user@example.com"},
		resendErr: errors.New("database unavailable"),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, "/account/resend-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account?resend=error" {
		t.Fatalf("Location = %q, want %q", location, "/account?resend=error")
	}
}

func TestRoutesResendVerificationRequiresCSRF(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodPost, "/account/resend-verification", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRoutesResendVerificationRequiresAuth(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, "/account/resend-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRoutesPublicResendVerificationForm(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/resend-verification", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "resend-form") {
		t.Fatalf("body = %q, want resend form marker", body)
	}
}

func TestRoutesPublicResendVerification(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/resend-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/resend-verification?status=sent" {
		t.Fatalf("Location = %q, want %q", location, "/resend-verification?status=sent")
	}
	if auth.publicResendCalls != 1 {
		t.Fatalf("public resend calls = %d, want 1", auth.publicResendCalls)
	}
	if auth.publicResendEmail != "user@example.com" {
		t.Fatalf("public resend email = %q, want %q", auth.publicResendEmail, "user@example.com")
	}
}

func TestRoutesPublicResendVerificationRejectsInvalidEmail(t *testing.T) {
	auth := &fakeAuthLookup{
		publicResendErr: services.ErrInvalidEmail,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"not-an-email"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/resend-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Enter a valid email address.") {
		t.Fatalf("body = %q, want invalid email message", rec.Body.String())
	}
}

func TestRoutesPublicResendVerificationHandlesError(t *testing.T) {
	auth := &fakeAuthLookup{
		publicResendErr: errors.New("database unavailable"),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, "/resend-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/resend-verification?status=error" {
		t.Fatalf("Location = %q, want %q", location, "/resend-verification?status=error")
	}
}

func TestRoutesPublicResendVerificationRequiresCSRF(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{"email": []string{"user@example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/resend-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRoutesPublicResendVerificationRedirectsAuthenticatedUser(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/resend-verification", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/account" {
		t.Fatalf("Location = %q, want %q", location, "/account")
	}
}

func TestRoutesAccountRejectsAnonymousUser(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/login?next=%2Faccount" {
		t.Fatalf("Location = %q, want %q", location, "/login?next=%2Faccount")
	}
}

func newAuthRouteTestServer(t *testing.T, auth authService) *Server {
	t.Helper()

	return &Server{
		db:     testDB(t),
		auth:   auth,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: testTemplates(t, map[string]string{
			"home.html":                `home {{ if .Authenticated }}Account Sign out {{ .User.Email }}{{ else }}Sign in Create account{{ end }}`,
			"register.html":            `register {{ .Error }} {{ with index .FieldErrors "email" }}{{ . }}{{ end }} {{ with index .FieldErrors "password" }}{{ . }}{{ end }} {{ with index .FieldErrors "confirm_password" }}{{ . }}{{ end }} {{ .Email }} {{ .PasswordMinLength }} {{ .CSRFToken }}`,
			"login.html":               `login {{ .Error }} {{ .CSRFToken }} {{ .Next }} /resend-verification`,
			"account.html":             `account {{ .User.Email }} {{ .CSRFToken }} {{ if not .User.EmailVerifiedAt.Valid }}resend-visible check-email-visible{{ end }} resend-status={{ .ResendStatus }}`,
			"confirm_email.html":       `confirm {{ if .Error }}{{ .Error }} {{ if .Authenticated }}Go to your account{{ else }}Sign in{{ end }}{{ else }}Email confirmed{{ end }}`,
			"resend_verification.html": `resend-form {{ .Error }} {{ with index .FieldErrors "email" }}{{ . }}{{ end }} {{ .Email }} resend-status={{ .ResendStatus }} {{ .CSRFToken }}`,
		}),
		passwordMinLength: 8,
	}
}

func cookieFromRecorder(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}

	t.Fatalf("missing %q cookie", name)
	return nil
}
