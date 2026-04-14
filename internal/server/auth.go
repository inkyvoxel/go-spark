package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const sessionCookieName = "session"

type authService interface {
	Login(context.Context, string, string) (db.User, db.Session, error)
	Logout(context.Context, string) error
	Register(context.Context, string, string) (db.User, error)
	UserBySessionToken(context.Context, string) (db.User, error)
}

type authContextKey struct{}

func currentUser(ctx context.Context) (db.User, bool) {
	user, ok := ctx.Value(authContextKey{}).(db.User)
	return user, ok
}

func contextWithUser(ctx context.Context, user db.User) context.Context {
	return context.WithValue(ctx, authContextKey{}, user)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, session db.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if errors.Is(err, http.ErrNoCookie) {
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		user, err := s.auth.UserBySessionToken(r.Context(), cookie.Value)
		if errors.Is(err, services.ErrInvalidSession) {
			clearSessionCookie(w, r)
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			s.logger.Error("load user by session", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r.WithContext(contextWithUser(r.Context(), user)))
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(r.Context()); !ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
