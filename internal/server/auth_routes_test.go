package server

import (
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
			"home.html":     `home {{ if .Authenticated }}Account Sign out {{ .User.Email }}{{ else }}Sign in Create account{{ end }}`,
			"register.html": `register {{ .Error }} {{ with index .FieldErrors "email" }}{{ . }}{{ end }} {{ with index .FieldErrors "password" }}{{ . }}{{ end }} {{ with index .FieldErrors "confirm_password" }}{{ . }}{{ end }} {{ .Email }} {{ .PasswordMinLength }} {{ .CSRFToken }}`,
			"login.html":    `login {{ .Error }} {{ .CSRFToken }} {{ .Next }}`,
			"account.html":  `account {{ .User.Email }} {{ .CSRFToken }}`,
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
