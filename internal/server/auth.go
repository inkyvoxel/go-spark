package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	sessionCookieName     = "session"
	resetCookieName       = "reset_token"
	resetCookiePath       = "/account/reset-password"
	resetCookieTTL        = 10 * time.Minute
	totpPendingCookieName = "totp_pending"
	totpPendingCookiePath = "/account/two-factor/challenge"
	totpPendingCookieTTL  = 5 * time.Minute
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
	// TOTP
	BeginTOTPSetup(context.Context, int64) (secret, otpAuthURI string, err error)
	GetTOTPSetupInfo(context.Context, int64) (secret, otpAuthURI string, err error)
	ConfirmTOTPSetup(context.Context, int64, string) ([]string, error)
	DisableTOTP(context.Context, int64, string) error
	GetTOTPStatus(context.Context, int64) (enabled bool, backupCodesRemaining int, err error)
	TOTPSetupState(context.Context, int64) (pending, enabled bool, err error)
	VerifyTOTPLogin(context.Context, int64, string) (services.AuthSession, error)
}

type authContextKey struct{}

func currentUser(ctx context.Context) (services.User, bool) {
	user, ok := ctx.Value(authContextKey{}).(services.User)
	return user, ok
}

func contextWithUser(ctx context.Context, user services.User) context.Context {
	return context.WithValue(ctx, authContextKey{}, user)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, session services.AuthSession, rememberMe bool) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	}
	if rememberMe {
		cookie.Expires = session.ExpiresAt
		cookie.MaxAge = sessionCookieMaxAge(session.ExpiresAt)
	}
	http.SetCookie(w, cookie)
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

func (s *Server) totpPendingKey() []byte {
	return deriveKey(s.csrfKey, "totp_pending")
}

// setTOTPPendingCookie stores a short-lived signed cookie carrying the userID
// and explicit remember-me choice so the TOTP challenge handler can complete
// the login without a DB lookup of the half-authenticated state.
type totpPendingLogin struct {
	UserID     int64
	RememberMe bool
}

func (s *Server) setTOTPPendingCookie(w http.ResponseWriter, r *http.Request, userID int64, rememberMe bool) {
	now := time.Now().UTC()
	payload := fmt.Sprintf("%d:%t:%d", userID, rememberMe, now.Unix())

	h := hmac.New(sha256.New, s.totpPendingKey())
	h.Write([]byte(payload))
	sig := h.Sum(nil)

	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(sig)

	http.SetCookie(w, &http.Cookie{
		Name:     totpPendingCookieName,
		Value:    value,
		Path:     totpPendingCookiePath,
		Expires:  now.Add(totpPendingCookieTTL),
		MaxAge:   int(totpPendingCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearTOTPPendingCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     totpPendingCookieName,
		Value:    "",
		Path:     totpPendingCookiePath,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
}

// totpPendingLoginFromCookie verifies the TOTP pending cookie signature and
// returns the embedded login state. Returns false if invalid or expired.
func (s *Server) totpPendingLoginFromCookie(r *http.Request) (totpPendingLogin, bool) {
	cookie, err := r.Cookie(totpPendingCookieName)
	if err != nil {
		return totpPendingLogin{}, false
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return totpPendingLogin{}, false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return totpPendingLogin{}, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return totpPendingLogin{}, false
	}

	h := hmac.New(sha256.New, s.totpPendingKey())
	h.Write(payloadBytes)
	if !hmac.Equal(sig, h.Sum(nil)) {
		return totpPendingLogin{}, false
	}

	payload := string(payloadBytes)
	fields := strings.Split(payload, ":")
	if len(fields) != 3 {
		return totpPendingLogin{}, false
	}

	userID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || userID <= 0 {
		return totpPendingLogin{}, false
	}

	rememberMe, err := strconv.ParseBool(fields[1])
	if err != nil {
		return totpPendingLogin{}, false
	}

	ts, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return totpPendingLogin{}, false
	}
	if time.Since(time.Unix(ts, 0)) > totpPendingCookieTTL {
		return totpPendingLogin{}, false
	}

	return totpPendingLogin{UserID: userID, RememberMe: rememberMe}, true
}
