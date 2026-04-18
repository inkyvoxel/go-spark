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
	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"

	_ "modernc.org/sqlite"
)

func TestRoutesLogin(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
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
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Account {
		t.Fatalf("Location = %q, want %q", location, paths.Account)
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

func TestRoutesLoginHTMXReturnsRedirectHeaderAndSession(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
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
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX redirect", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != paths.Account {
		t.Fatalf("HX-Redirect = %q, want %q", redirect, paths.Account)
	}
	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.Value != "session-token" {
		t.Fatalf("session cookie = %q, want %q", session.Value, "session-token")
	}
}

func TestRoutesLoginHTMXRejectsInvalidCredentials(t *testing.T) {
	auth := &fakeAuthLookup{
		loginErr: services.ErrInvalidCredentials,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		"password":    []string{"wrong-password"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != "" {
		t.Fatalf("HX-Redirect = %q, want empty for invalid credentials", redirect)
	}
	if !strings.Contains(rec.Body.String(), "Email or password is not correct.") {
		t.Fatalf("body = %q, want invalid credentials error", rec.Body.String())
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
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
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
		user: verifiedRouteUser(),
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
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
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
		user: verifiedRouteUser(),
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
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Account {
		t.Fatalf("Location = %q, want %q", location, paths.Account)
	}
}

func TestRoutesLoginFormIncludesSafeNextPath(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.Login+"?next=%2Faccount", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), paths.Account) {
		t.Fatalf("body = %q, want safe next path", rec.Body.String())
	}
}

func TestRoutesLoginFormOmitsUnsafeNextPath(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.Login+"?next=https%3A%2F%2Fevil.example", nil)
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

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), paths.ResendVerification) {
		t.Fatalf("body = %q, want resend verification link", rec.Body.String())
	}
}

func TestRoutesLoginFormShowsForgotPasswordLink(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), paths.ForgotPassword) {
		t.Fatalf("body = %q, want forgot password link", rec.Body.String())
	}
}

func TestRoutesLoginFormShowsPasswordChangedStatus(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, loginPathWithStatusChanged, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "login-status=password-changed") {
		t.Fatalf("body = %q, want password-changed login status", rec.Body.String())
	}
}

func TestRoutesLoginRedirectsAuthenticatedUserToAccount(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Account {
		t.Fatalf("Location = %q, want %q", location, paths.Account)
	}
}

func TestRoutesLoginRedirectsAuthenticatedUnverifiedUserToVerifyEmail(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.Login, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
	}
}

func TestRoutesLoginRedirectsUnverifiedUserToVerifyEmail(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
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
	req := httptest.NewRequest(http.MethodPost, paths.Register, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
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

func TestRoutesRegisterHTMXReturnsRedirectHeaderAndSession(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, paths.Register, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX redirect", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != paths.VerifyEmail {
		t.Fatalf("HX-Redirect = %q, want %q", redirect, paths.VerifyEmail)
	}
	if !auth.registered {
		t.Fatal("Register() was not called")
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
	req := httptest.NewRequest(http.MethodPost, paths.Register, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if auth.registered {
		t.Fatal("Register() was called")
	}
	if !strings.Contains(rec.Body.String(), "Passwords do not match.") {
		t.Fatalf("body = %q, want confirmation error", rec.Body.String())
	}
}

func TestRoutesRegisterHTMXRejectsMismatchedPasswordConfirmation(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":            []string{"new@example.com"},
		"password":         []string{"password"},
		"confirm_password": []string{"different"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.Register, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != "" {
		t.Fatalf("HX-Redirect = %q, want empty for validation error", redirect)
	}
	if !strings.Contains(rec.Body.String(), "Passwords do not match.") {
		t.Fatalf("body = %q, want confirmation error", rec.Body.String())
	}
}

func TestRoutesRegisterHTMXShowsServiceValidationErrors(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, paths.Register, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != "" {
		t.Fatalf("HX-Redirect = %q, want empty for validation error", redirect)
	}
	if !strings.Contains(rec.Body.String(), "An account with this email already exists.") {
		t.Fatalf("body = %q, want duplicate email error", rec.Body.String())
	}
}

func TestRoutesRegisterValidatesRequiredFields(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, paths.Register, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
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
	req := httptest.NewRequest(http.MethodPost, paths.Register, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
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

	req := httptest.NewRequest(http.MethodGet, paths.ConfirmEmail+"?token=raw-token", nil)
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

	req := httptest.NewRequest(http.MethodGet, paths.ConfirmEmail+"?token=bad-token", nil)
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

	req := httptest.NewRequest(http.MethodGet, paths.ConfirmEmail+"?token=bad-token", nil)
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

	req := httptest.NewRequest(http.MethodGet, paths.ConfirmEmail+"?token=raw-token", nil)
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
	req := httptest.NewRequest(http.MethodPost, paths.Logout, strings.NewReader(form.Encode()))
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
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.Account, nil)
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

	req := httptest.NewRequest(http.MethodGet, paths.Account, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
	}
}

func TestRoutesVerifyEmailRequiresAuth(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.VerifyEmail, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Login+"?next=%2Faccount%2Fverify-email" {
		t.Fatalf("Location = %q, want %q", location, paths.Login+"?next=%2Faccount%2Fverify-email")
	}
}

func TestRoutesVerifyEmailShowsInterstitialForUnverifiedUser(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.VerifyEmail, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "verify-email") {
		t.Fatalf("body = %q, want verify-email marker", rec.Body.String())
	}
}

func TestRoutesVerifyEmailRedirectsVerifiedUserToAccount(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.VerifyEmail, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Account {
		t.Fatalf("Location = %q, want %q", location, paths.Account)
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

	req := httptest.NewRequest(http.MethodGet, paths.Account, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "change-password-visible") {
		t.Fatalf("body = %q, want change password form", body)
	}
}

func TestRoutesResendVerification(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, paths.VerifyEmailResend, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != withQueryParam(paths.VerifyEmail, queryKeyResend, statusSent) {
		t.Fatalf("Location = %q, want %q", location, withQueryParam(paths.VerifyEmail, queryKeyResend, statusSent))
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
	req := httptest.NewRequest(http.MethodPost, paths.VerifyEmailResend, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != withQueryParam(paths.VerifyEmail, queryKeyResend, statusError) {
		t.Fatalf("Location = %q, want %q", location, withQueryParam(paths.VerifyEmail, queryKeyResend, statusError))
	}
}

func TestRoutesResendVerificationHTMXReturnsFragment(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, paths.VerifyEmailResend, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "resend-status=sent") {
		t.Fatalf("body = %q, want resend-status=sent", rec.Body.String())
	}
}

func TestRoutesResendVerificationHTMXReturnsErrorFragment(t *testing.T) {
	auth := &fakeAuthLookup{
		user:      db.User{ID: 1, Email: "user@example.com"},
		resendErr: errors.New("database unavailable"),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{csrfFieldName: []string{"csrf"}}
	req := httptest.NewRequest(http.MethodPost, paths.VerifyEmailResend, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "resend-status=error") {
		t.Fatalf("body = %q, want resend-status=error", rec.Body.String())
	}
}

func TestRoutesResendVerificationRequiresCSRF(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodPost, paths.VerifyEmailResend, strings.NewReader(url.Values{}.Encode()))
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
	req := httptest.NewRequest(http.MethodPost, paths.VerifyEmailResend, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRoutesChangePassword(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"current_password": []string{"old-password"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != loginPathWithStatusChanged {
		t.Fatalf("Location = %q, want %q", location, loginPathWithStatusChanged)
	}
	if !auth.changePasswordCalled {
		t.Fatal("ChangePassword() was not called")
	}
	if auth.changePasswordUser.ID != auth.user.ID {
		t.Fatalf("change password user ID = %d, want %d", auth.changePasswordUser.ID, auth.user.ID)
	}
	if auth.changePasswordOld != "old-password" || auth.changePasswordNew != "new-password" {
		t.Fatalf("change password values = %q/%q", auth.changePasswordOld, auth.changePasswordNew)
	}
	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.MaxAge != -1 {
		t.Fatalf("session MaxAge = %d, want %d", session.MaxAge, -1)
	}
}

func TestRoutesChangePasswordValidatesFields(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if auth.changePasswordCalled {
		t.Fatal("ChangePassword() was called")
	}
	body := rec.Body.String()
	for _, want := range []string{"Enter your current password.", "Enter a password.", "Confirm your password."} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
}

func TestRoutesChangePasswordHTMXValidatesFields(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != "" {
		t.Fatalf("HX-Redirect = %q, want empty for validation error", redirect)
	}
	body := rec.Body.String()
	for _, want := range []string{"change-password-visible", "Enter your current password.", "Enter a password.", "Confirm your password."} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
}

func TestRoutesChangePasswordHTMXRedirectsOnSuccess(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"current_password": []string{"old-password"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX redirect", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != loginPathWithStatusChanged {
		t.Fatalf("HX-Redirect = %q, want %q", redirect, loginPathWithStatusChanged)
	}
	session := cookieFromRecorder(t, rec, sessionCookieName)
	if session.MaxAge != -1 {
		t.Fatalf("session MaxAge = %d, want %d", session.MaxAge, -1)
	}
}

func TestRoutesChangePasswordRejectsIncorrectCurrentPassword(t *testing.T) {
	auth := &fakeAuthLookup{
		user:              verifiedRouteUser(),
		changePasswordErr: services.ErrCurrentPasswordIncorrect,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"current_password": []string{"wrong-password"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Current password is not correct.") {
		t.Fatalf("body = %q, want incorrect-current-password error", body)
	}
}

func TestRoutesChangePasswordRequiresCSRF(t *testing.T) {
	auth := &fakeAuthLookup{
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"current_password": []string{"old-password"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRoutesChangePasswordRequiresAuth(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{
		"current_password": []string{"old-password"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRoutesChangePasswordRequiresVerifiedEmail(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"current_password": []string{"old-password"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ChangePassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if auth.changePasswordCalled {
		t.Fatal("ChangePassword() was called")
	}
}

func TestRoutesPublicResendVerificationForm(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.ResendVerification, nil)
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
	req := httptest.NewRequest(http.MethodPost, paths.ResendVerification, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != withQueryParam(paths.ResendVerification, queryKeyStatus, statusSent) {
		t.Fatalf("Location = %q, want %q", location, withQueryParam(paths.ResendVerification, queryKeyStatus, statusSent))
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
	req := httptest.NewRequest(http.MethodPost, paths.ResendVerification, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
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
	req := httptest.NewRequest(http.MethodPost, paths.ResendVerification, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != withQueryParam(paths.ResendVerification, queryKeyStatus, statusError) {
		t.Fatalf("Location = %q, want %q", location, withQueryParam(paths.ResendVerification, queryKeyStatus, statusError))
	}
}

func TestRoutesPublicResendVerificationHTMXReturnsFragment(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResendVerification, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "resend-status=sent") {
		t.Fatalf("body = %q, want resend-status=sent", rec.Body.String())
	}
}

func TestRoutesPublicResendVerificationHTMXReturnsErrorFragment(t *testing.T) {
	auth := &fakeAuthLookup{
		publicResendErr: errors.New("database unavailable"),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResendVerification, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "resend-status=error") {
		t.Fatalf("body = %q, want resend-status=error", rec.Body.String())
	}
}

func TestRoutesPublicResendVerificationHTMXRejectsInvalidEmail(t *testing.T) {
	auth := &fakeAuthLookup{
		publicResendErr: services.ErrInvalidEmail,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"not-an-email"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResendVerification, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "Enter a valid email address.") {
		t.Fatalf("body = %q, want invalid email message", rec.Body.String())
	}
}

func TestRoutesPublicResendVerificationRequiresCSRF(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{"email": []string{"user@example.com"}}
	req := httptest.NewRequest(http.MethodPost, paths.ResendVerification, strings.NewReader(form.Encode()))
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

	req := httptest.NewRequest(http.MethodGet, paths.ResendVerification, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
	}
}

func TestRoutesForgotPasswordForm(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.ForgotPassword, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "forgot-form") {
		t.Fatalf("body = %q, want forgot password form marker", rec.Body.String())
	}
}

func TestRoutesForgotPassword(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ForgotPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != withQueryParam(paths.ForgotPassword, queryKeyStatus, statusSent) {
		t.Fatalf("Location = %q, want %q", location, withQueryParam(paths.ForgotPassword, queryKeyStatus, statusSent))
	}
	if auth.requestResetEmail != "user@example.com" {
		t.Fatalf("request reset email = %q, want %q", auth.requestResetEmail, "user@example.com")
	}
}

func TestRoutesForgotPasswordRejectsInvalidEmail(t *testing.T) {
	auth := &fakeAuthLookup{
		requestResetErr: services.ErrInvalidEmail,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"not-an-email"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ForgotPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "Enter a valid email address.") {
		t.Fatalf("body = %q, want invalid email message", rec.Body.String())
	}
}

func TestRoutesForgotPasswordHandlesError(t *testing.T) {
	auth := &fakeAuthLookup{
		requestResetErr: errors.New("database unavailable"),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ForgotPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != withQueryParam(paths.ForgotPassword, queryKeyStatus, statusError) {
		t.Fatalf("Location = %q, want %q", location, withQueryParam(paths.ForgotPassword, queryKeyStatus, statusError))
	}
}

func TestRoutesForgotPasswordHTMXReturnsFragment(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ForgotPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "forgot-status=sent") {
		t.Fatalf("body = %q, want forgot-status=sent", rec.Body.String())
	}
}

func TestRoutesForgotPasswordHTMXReturnsErrorFragment(t *testing.T) {
	auth := &fakeAuthLookup{
		requestResetErr: errors.New("database unavailable"),
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"user@example.com"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ForgotPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "forgot-status=error") {
		t.Fatalf("body = %q, want forgot-status=error", rec.Body.String())
	}
}

func TestRoutesForgotPasswordHTMXRejectsInvalidEmail(t *testing.T) {
	auth := &fakeAuthLookup{
		requestResetErr: services.ErrInvalidEmail,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"email":       []string{"not-an-email"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ForgotPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if !strings.Contains(rec.Body.String(), "Enter a valid email address.") {
		t.Fatalf("body = %q, want invalid email message", rec.Body.String())
	}
}

func TestRoutesForgotPasswordRequiresCSRF(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{"email": []string{"user@example.com"}}
	req := httptest.NewRequest(http.MethodPost, paths.ForgotPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRoutesForgotPasswordRedirectsAuthenticatedUser(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.ForgotPassword, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.VerifyEmail {
		t.Fatalf("Location = %q, want %q", location, paths.VerifyEmail)
	}
}

func TestRoutesResetPasswordForm(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.ResetPassword+"?token=raw-token", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if auth.validateResetToken != "raw-token" {
		t.Fatalf("validate reset token = %q, want %q", auth.validateResetToken, "raw-token")
	}
	if !strings.Contains(rec.Body.String(), "reset-form") {
		t.Fatalf("body = %q, want reset form marker", rec.Body.String())
	}
}

func TestRoutesResetPasswordFormRejectsInvalidToken(t *testing.T) {
	auth := &fakeAuthLookup{
		validateResetErr: services.ErrInvalidPasswordResetToken,
	}
	srv := newAuthRouteTestServer(t, auth)

	req := httptest.NewRequest(http.MethodGet, paths.ResetPassword+"?token=bad-token", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid or has expired") {
		t.Fatalf("body = %q, want invalid reset token message", rec.Body.String())
	}
}

func TestRoutesResetPassword(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"token":            []string{"raw-token"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResetPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != loginPathWithStatusChanged {
		t.Fatalf("Location = %q, want %q", location, loginPathWithStatusChanged)
	}
	if auth.resetToken != "raw-token" || auth.resetNewPassword != "new-password" {
		t.Fatalf("reset inputs = %q/%q", auth.resetToken, auth.resetNewPassword)
	}
}

func TestRoutesResetPasswordValidatesFields(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{
		"token":       []string{"raw-token"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResetPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	body := rec.Body.String()
	for _, want := range []string{"Enter a password.", "Confirm your password."} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
}

func TestRoutesResetPasswordHTMXValidatesFields(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{
		"token":       []string{"raw-token"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResetPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX fragment", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != "" {
		t.Fatalf("HX-Redirect = %q, want empty for validation error", redirect)
	}
	body := rec.Body.String()
	for _, want := range []string{"reset-form", "Enter a password.", "Confirm your password."} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}
}

func TestRoutesResetPasswordHTMXRedirectsOnSuccess(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"token":            []string{"raw-token"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResetPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty for HTMX redirect", location)
	}
	if redirect := rec.Header().Get("HX-Redirect"); redirect != loginPathWithStatusChanged {
		t.Fatalf("HX-Redirect = %q, want %q", redirect, loginPathWithStatusChanged)
	}
	if auth.resetToken != "raw-token" || auth.resetNewPassword != "new-password" {
		t.Fatalf("reset inputs = %q/%q", auth.resetToken, auth.resetNewPassword)
	}
}

func TestRoutesResetPasswordRejectsInvalidToken(t *testing.T) {
	auth := &fakeAuthLookup{
		resetErr: services.ErrInvalidPasswordResetToken,
	}
	srv := newAuthRouteTestServer(t, auth)

	form := url.Values{
		"token":            []string{"bad-token"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
		csrfFieldName:      []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResetPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid or has expired") {
		t.Fatalf("body = %q, want invalid reset token message", rec.Body.String())
	}
}

func TestRoutesResetPasswordRequiresCSRF(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	form := url.Values{
		"token":            []string{"raw-token"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.ResetPassword, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRoutesAccountRejectsAnonymousUser(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, paths.Account, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != paths.Login+"?next=%2Faccount" {
		t.Fatalf("Location = %q, want %q", location, paths.Login+"?next=%2Faccount")
	}
}

func newAuthRouteTestServer(t *testing.T, auth authService) *Server {
	t.Helper()

	return &Server{
		db:     testDB(t),
		auth:   auth,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: testTemplates(t, map[string]string{
			templateHome:               `home {{ if .Authenticated }}Account Sign out {{ .User.Email }}{{ else }}Sign in Create account{{ end }}`,
			templateRegister:           `{{ define "content" }}{{ template "register_form_section" . }}{{ end }}{{ define "register_form_section" }}register {{ .Error }} {{ with index .FieldErrors "email" }}{{ . }}{{ end }} {{ with index .FieldErrors "password" }}{{ . }}{{ end }} {{ with index .FieldErrors "confirm_password" }}{{ . }}{{ end }} {{ .Email }} {{ .PasswordMinLength }} {{ .CSRFToken }}{{ end }}`,
			templateLogin:              `{{ define "content" }}{{ template "login_form_section" . }} ` + paths.ForgotPassword + ` ` + paths.ResendVerification + `{{ end }}{{ define "login_form_section" }}login {{ .Error }} {{ with index .FieldErrors "email" }}{{ . }}{{ end }} {{ with index .FieldErrors "password" }}{{ . }}{{ end }} {{ .CSRFToken }} {{ .Next }} login-status={{ .LoginStatus }}{{ end }}`,
			templateAccount:            `{{ define "content" }}account {{ .User.Email }} {{ .CSRFToken }} {{ template "account_change_password_section" . }}{{ end }}{{ define "account_change_password_section" }}change-password-visible {{ .Error }} {{ with index .FieldErrors "current_password" }}{{ . }}{{ end }} {{ with index .FieldErrors "new_password" }}{{ . }}{{ end }} {{ with index .FieldErrors "confirm_password" }}{{ . }}{{ end }}{{ end }}`,
			templateConfirmEmail:       `confirm {{ if .Error }}{{ .Error }} {{ if .Authenticated }}Go to your account{{ else }}Sign in{{ end }}{{ else }}Email confirmed{{ end }}`,
			templateForgotPassword:     `{{ define "content" }}{{ template "forgot_password_form_section" . }}{{ end }}{{ define "forgot_password_form_section" }}forgot-form {{ .Error }} {{ with index .FieldErrors "email" }}{{ . }}{{ end }} {{ .Email }} forgot-status={{ .ForgotPasswordStatus }} {{ .CSRFToken }}{{ end }}`,
			templateResetPassword:      `{{ define "content" }}{{ if .ResetTokenInvalid }}{{ .Error }}{{ else }}{{ template "reset_password_form_section" . }}{{ end }}{{ end }}{{ define "reset_password_form_section" }}reset-form {{ .Error }} {{ with index .FieldErrors "new_password" }}{{ . }}{{ end }} {{ with index .FieldErrors "confirm_password" }}{{ . }}{{ end }} token={{ .ResetToken }} {{ if .ResetTokenInvalid }}invalid or has expired{{ end }} {{ .CSRFToken }}{{ end }}`,
			templateResendVerification: `{{ define "content" }}{{ template "resend_verification_form_section" . }}{{ end }}{{ define "resend_verification_form_section" }}resend-form {{ .Error }} {{ with index .FieldErrors "email" }}{{ . }}{{ end }} {{ .Email }} resend-status={{ .ResendStatus }} {{ .CSRFToken }}{{ end }}`,
			templateVerifyEmail:        `{{ define "content" }}verify-email {{ .User.Email }} {{ template "verify_email_resend_section" . }}{{ end }}{{ define "verify_email_resend_section" }}resend-status={{ .ResendStatus }}{{ end }}`,
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

func verifiedRouteUser() db.User {
	return db.User{
		ID:    1,
		Email: "user@example.com",
		EmailVerifiedAt: sql.NullTime{
			Time:  time.Now().UTC(),
			Valid: true,
		},
	}
}
