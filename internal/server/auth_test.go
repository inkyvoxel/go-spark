package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	db "go-starter/internal/db/generated"
	"go-starter/internal/services"
)

func TestRequireAuthAddsCurrentUserToContext(t *testing.T) {
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

	srv.requireAuth(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if auth.token != "session-token" {
		t.Fatalf("auth lookup token = %q, want %q", auth.token, "session-token")
	}
}

func TestRequireAuthRejectsMissingCookie(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()

	srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthRejectsInvalidSession(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{
		err: services.ErrInvalidSession,
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bad-token"})
	rec := httptest.NewRecorder()

	srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthHandlesLookupError(t *testing.T) {
	srv := newAuthMiddlewareTestServer(&fakeAuthLookup{
		err: errors.New("database unavailable"),
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCurrentUserReturnsFalseWhenMissing(t *testing.T) {
	if _, ok := currentUser(context.Background()); ok {
		t.Fatal("currentUser() ok = true, want false")
	}
}

type fakeAuthLookup struct {
	user          db.User
	token         string
	err           error
	registered    bool
	registerEmail string
	registerPass  string
	registerErr   error
	loginEmail    string
	loginPass     string
	loginSession  db.Session
	loginErr      error
	logoutToken   string
	logoutErr     error
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

func newAuthMiddlewareTestServer(auth authService) *Server {
	return &Server{
		auth:   auth,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}
