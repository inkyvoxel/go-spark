package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	sessionCookieName = "session"
	resetCookieName   = "reset_token"
	resetCookiePath   = "/account/reset-password"
	resetCookieTTL    = 10 * time.Minute
)

type authService interface {
	RequestEmailChange(context.Context, int64, string, string) error
	ConfirmEmailChange(context.Context, string) (services.User, error)
	ChangePassword(context.Context, int64, string, string) error
	ListManagedSessions(context.Context, int64, string) ([]services.ManagedSession, error)
	RevokeOtherSessions(context.Context, int64, string) error
	RevokeSessionByID(context.Context, int64, string, int64) error
	Login(context.Context, string, string) (services.User, services.AuthSession, error)
	Logout(context.Context, string) error
	RequestPasswordReset(context.Context, string) error
	Register(context.Context, string, string) (services.User, error)
	ResetPasswordWithToken(context.Context, string, string) error
	ResendVerificationEmailByAddress(context.Context, string) error
	ResendVerificationEmail(context.Context, int64) error
	UserBySessionToken(context.Context, string) (services.User, error)
	ValidatePasswordResetToken(context.Context, string) error
	VerifyEmail(context.Context, string) (services.User, error)
	DeleteAccount(context.Context, int64, string) error
}

type authContextKey struct{}

func currentUser(ctx context.Context) (services.User, bool) {
	user, ok := ctx.Value(authContextKey{}).(services.User)
	return user, ok
}

func contextWithUser(ctx context.Context, user services.User) context.Context {
	return context.WithValue(ctx, authContextKey{}, user)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, session services.AuthSession) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		MaxAge:   sessionCookieMaxAge(session.ExpiresAt),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func sessionCookieMaxAge(expiresAt time.Time) int {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		return 1
	}
	return maxAge
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

func (s *Server) setResetCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     resetCookieName,
		Value:    strings.TrimSpace(token),
		Path:     resetCookiePath,
		Expires:  time.Now().UTC().Add(resetCookieTTL),
		MaxAge:   int(resetCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearResetCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     resetCookieName,
		Value:    "",
		Path:     resetCookiePath,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func resetTokenFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(resetCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func sessionTokenFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
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
			s.clearCSRFCookie(w, r)
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			s.loggerForRequest(r).Error("load user by session", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		ctx := contextWithUser(r.Context(), user)
		markRequestAuthenticated(ctx)
		next.ServeHTTP(w, r.WithContext(ctx))
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
