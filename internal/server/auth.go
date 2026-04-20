package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const sessionCookieName = "session"

type authService interface {
	RequestEmailChange(context.Context, db.User, string, string) error
	ConfirmEmailChange(context.Context, string) (db.User, error)
	ChangePassword(context.Context, db.User, string, string) error
	Login(context.Context, string, string) (db.User, services.AuthSession, error)
	Logout(context.Context, string) error
	RequestPasswordReset(context.Context, string) error
	Register(context.Context, string, string) (db.User, error)
	ResetPasswordWithToken(context.Context, string, string) error
	ResendVerificationEmailByAddress(context.Context, string) error
	ResendVerificationEmail(context.Context, db.User) error
	UserBySessionToken(context.Context, string) (db.User, error)
	ValidatePasswordResetToken(context.Context, string) error
	VerifyEmail(context.Context, string) (db.User, error)
}

type authContextKey struct{}

func currentUser(ctx context.Context) (db.User, bool) {
	user, ok := ctx.Value(authContextKey{}).(db.User)
	return user, ok
}

func contextWithUser(ctx context.Context, user db.User) context.Context {
	return context.WithValue(ctx, authContextKey{}, user)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, session services.AuthSession) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) secureCookie(r *http.Request) bool {
	return s.cookieSecure || r.TLS != nil
}

func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			s.clearSessionCookie(w, r)
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
			if r.Method == http.MethodGet {
				redirectToLogin(w, r)
				return
			}
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAnonymous(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := currentUser(r.Context()); ok {
			if s.emailVerificationPolicy.UserIsVerified(user) {
				http.Redirect(w, r, paths.Account, http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, paths.VerifyEmail, http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireVerifiedAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r.Context())
		if !ok {
			if r.Method == http.MethodGet {
				redirectToLogin(w, r)
				return
			}
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		if !s.emailVerificationPolicy.UserIsVerified(user) {
			if r.Method == http.MethodGet {
				http.Redirect(w, r, paths.VerifyEmail, http.StatusSeeOther)
				return
			}
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func emailVerificationPolicy(policy services.EmailVerificationPolicy) services.EmailVerificationPolicy {
	if policy == nil {
		return services.DefaultEmailVerificationPolicy()
	}
	return policy
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	loginURL := paths.Login
	next := safeRedirectPath(r.URL.RequestURI())
	if next != "" {
		loginURL += "?next=" + url.QueryEscape(next)
	}
	http.Redirect(w, r, loginURL, http.StatusSeeOther)
}

func safeRedirectPath(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, `\`) || strings.HasPrefix(value, `/\`) {
		return ""
	}

	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if u.IsAbs() || u.Host != "" {
		return ""
	}
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return ""
	}
	if strings.Contains(u.Path, `\`) {
		return ""
	}

	return u.RequestURI()
}
