package server

import (
	"database/sql"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	db "go-starter/internal/db/generated"

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
		"email":       []string{"new@example.com"},
		"password":    []string{"password"},
		csrfFieldName: []string{"csrf"},
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

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func newAuthRouteTestServer(t *testing.T, auth authService) *Server {
	t.Helper()

	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return &Server{
		db:     conn,
		auth:   auth,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: template.Must(template.New("test").Parse(`
			{{ define "home.html" }}home{{ end }}
			{{ define "register.html" }}register {{ .Error }} {{ .CSRFToken }}{{ end }}
			{{ define "login.html" }}login {{ .Error }} {{ .CSRFToken }}{{ end }}
			{{ define "account.html" }}account {{ .User.Email }} {{ .CSRFToken }}{{ end }}
		`)),
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
