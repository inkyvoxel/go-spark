package server

import (
	"context"
	"database/sql"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	appassets "github.com/inkyvoxel/go-spark"
	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	maxRequestBodyBytes = 64 * 1024
	cspHeaderValue      = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'self'"
	cacheControlNoStore = "no-store"
	pragmaNoCache       = "no-cache"
	expiresImmediately  = "0"
)

type Server struct {
	db                      *sql.DB
	auth                    authService
	emailVerificationPolicy services.EmailVerificationPolicy
	logger                  *slog.Logger
	templates               map[string]*template.Template
	cookieSecure            bool
	appBaseOrigin           string
	passwordMinLength       int
	csrfSigningKey          []byte
	rateLimiter             rateLimitStore
	rateLimitPolicies       RateLimitPolicies
}

type Options struct {
	DB                      *sql.DB
	Auth                    authService
	EmailVerificationPolicy services.EmailVerificationPolicy
	Logger                  *slog.Logger
	CookieSecure            bool
	AppBaseURL              string
	CSRFSigningKey          string
	PasswordMinLength       int
	RateLimitPolicies       RateLimitPolicies
}

func New(opts Options) *Server {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Auth == nil {
		panic("server auth service is required")
	}

	passwordMinLength := opts.PasswordMinLength
	if passwordMinLength == 0 {
		passwordMinLength = services.DefaultPasswordMinLength
	}
	csrfSigningKey := []byte(strings.TrimSpace(opts.CSRFSigningKey))
	appBaseOrigin := normalizeOrigin(opts.AppBaseURL)

	return &Server{
		db:                      opts.DB,
		auth:                    opts.Auth,
		emailVerificationPolicy: emailVerificationPolicy(opts.EmailVerificationPolicy),
		logger:                  logger,
		templates:               mustParseTemplates(),
		cookieSecure:            opts.CookieSecure,
		appBaseOrigin:           appBaseOrigin,
		passwordMinLength:       passwordMinLength,
		csrfSigningKey:          csrfSigningKey,
		rateLimiter:             newInMemoryRateLimiter(),
		rateLimitPolicies:       rateLimitPoliciesWithDefaults(opts.RateLimitPolicies),
	}
}

func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}

func mustParseTemplates() map[string]*template.Template {
	templates, err := parseTemplates()
	if err != nil {
		panic(err)
	}

	return templates
}

func parseTemplates() (map[string]*template.Template, error) {
	pages := map[string]string{
		templateNotFound:           "404.html",
		templateAccount:            path.Join("account", "account.html"),
		templateChangeEmail:        path.Join("account", "change_email.html"),
		templateChangePassword:     path.Join("account", "change_password.html"),
		templateConfirmEmail:       path.Join("account", "confirm_email.html"),
		templateConfirmEmailChange: path.Join("account", "confirm_email_change.html"),
		templateForgotPassword:     path.Join("account", "forgot_password.html"),
		templateHome:               "home.html",
		templateLogin:              path.Join("account", "login.html"),
		templateResetPassword:      path.Join("account", "reset_password.html"),
		templateRegister:           path.Join("account", "register.html"),
		templateResendVerification: path.Join("account", "resend_verification.html"),
		templateVerifyEmail:        path.Join("account", "verify_email.html"),
	}
	templates := make(map[string]*template.Template, len(pages))
	layout := path.Join("templates", templateLayout)
	partials := []string{
		path.Join("templates", templateBreadcrumb),
	}

	for name, filePath := range pages {
		files := append([]string{layout}, partials...)
		files = append(files, path.Join("templates", filePath))
		parsed, err := template.ParseFS(appassets.FS, files...)
		if err != nil {
			return nil, err
		}
		templates[name] = parsed
	}

	return templates, nil
}

func (s *Server) Routes() http.Handler {
	s.ensureRateLimiting()

	mux := http.NewServeMux()
	dynamic := http.NewServeMux()

	mux.HandleFunc(route(http.MethodGet, paths.Healthz), s.healthz)
	mux.HandleFunc(route(http.MethodGet, paths.Readyz), s.readyz)
	mux.Handle(route(http.MethodGet, paths.StaticPrefix), staticFileHandler())

	// Register new protected pages with requireAuth and anonymous-only pages with requireAnonymous.
	dynamic.Handle(route(http.MethodGet, paths.Register), s.requireAnonymous(http.HandlerFunc(s.registerForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.Register),
		s.requireAnonymous(
			s.withRateLimit("register", s.rateLimitPolicies.Register, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.register)),
		),
	)
	dynamic.HandleFunc(route(http.MethodGet, paths.ConfirmEmail), s.confirmEmail)
	dynamic.HandleFunc(route(http.MethodGet, paths.ConfirmEmailChange), s.confirmEmailChange)
	dynamic.Handle(route(http.MethodGet, paths.ForgotPassword), s.requireAnonymous(http.HandlerFunc(s.forgotPasswordForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ForgotPassword),
		s.requireAnonymous(
			s.withRateLimit("forgot-password", s.rateLimitPolicies.ForgotPassword, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.forgotPassword)),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.Login), s.requireAnonymous(http.HandlerFunc(s.loginForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.Login),
		s.requireAnonymous(
			s.withRateLimit("login", s.rateLimitPolicies.Login, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.login)),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.ResetPassword), s.requireAnonymous(http.HandlerFunc(s.resetPasswordForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ResetPassword),
		s.requireAnonymous(
			s.withRateLimit("reset-password", s.rateLimitPolicies.ResetPassword, rateLimitKeyByIPAndResetTokenCookie(), http.HandlerFunc(s.resetPassword)),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.ResendVerification), s.requireAnonymous(http.HandlerFunc(s.resendVerificationForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ResendVerification),
		s.requireAnonymous(
			s.withRateLimit("resend-verification-public", s.rateLimitPolicies.PublicResendVerification, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.resendVerificationPublic)),
		),
	)
	dynamic.Handle(route(http.MethodPost, paths.Logout), s.requireAuth(http.HandlerFunc(s.logout)))
	dynamic.Handle(route(http.MethodGet, paths.VerifyEmail), s.requireAuth(http.HandlerFunc(s.verifyEmail)))
	dynamic.Handle(route(http.MethodGet, paths.Account), s.requireVerifiedAuth(http.HandlerFunc(s.account)))
	dynamic.Handle(route(http.MethodGet, paths.ChangeEmail), s.requireVerifiedAuth(http.HandlerFunc(s.changeEmailForm)))
	dynamic.Handle(route(http.MethodGet, paths.ChangePassword), s.requireVerifiedAuth(http.HandlerFunc(s.changePasswordForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.VerifyEmailResend),
		s.requireAuth(
			s.withRateLimit("resend-verification-account", s.rateLimitPolicies.AccountResendVerification, rateLimitKeyByIPAndUser(), http.HandlerFunc(s.resendVerification)),
		),
	)
	dynamic.Handle(
		route(http.MethodPost, paths.ChangePassword),
		s.requireVerifiedAuth(
			s.withRateLimit("change-password", s.rateLimitPolicies.ChangePassword, rateLimitKeyByIPAndUser(), http.HandlerFunc(s.changePassword)),
		),
	)
	dynamic.Handle(
		route(http.MethodPost, paths.ChangeEmail),
		s.requireVerifiedAuth(
			s.withRateLimit("change-email", s.rateLimitPolicies.ChangeEmail, rateLimitKeyByIPAndUser(), http.HandlerFunc(s.changeEmail)),
		),
	)
	dynamic.Handle(
		route(http.MethodPost, paths.AccountSessionsRevoke),
		s.requireVerifiedAuth(
			s.withRateLimit("revoke-session", s.rateLimitPolicies.RevokeSession, rateLimitKeyByIPAndUser(), http.HandlerFunc(s.revokeSession)),
		),
	)
	dynamic.Handle(
		route(http.MethodPost, paths.AccountSessionsRevokeOthers),
		s.requireVerifiedAuth(
			s.withRateLimit("revoke-other-sessions", s.rateLimitPolicies.RevokeOtherSessions, rateLimitKeyByIPAndUser(), http.HandlerFunc(s.revokeOtherSessions)),
		),
	)
	dynamic.HandleFunc(route(http.MethodGet, "/{$}"), s.home)
	dynamic.HandleFunc(route(http.MethodGet, "/{path...}"), s.notFoundPage)

	mux.Handle(paths.Home, s.cacheControlHeaders(s.securityHeaders(s.limitRequestBody(s.csrf(s.loadSession(dynamic))))))

	return s.withRequestID(s.logRequests(mux))
}

func route(method, path string) string {
	return method + " " + path
}

func staticFileHandler() http.Handler {
	fileServer := http.StripPrefix(paths.StaticPrefix, http.FileServerFS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent directory listing from exposing static tree contents.
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, templateHome, s.newTemplateData(r, "Go Spark"))
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writePlaintext(w, http.StatusOK, "ok")
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if !s.isReady(r.Context()) {
		writePlaintext(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	writePlaintext(w, http.StatusOK, "ok")
}

func (s *Server) isReady(ctx context.Context) bool {
	if s.db == nil {
		return false
	}
	return s.db.PingContext(ctx) == nil
}

func writePlaintext(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (s *Server) notFoundPage(w http.ResponseWriter, r *http.Request) {
	if allow, ok := postOnlyAllowForPath(r.URL.Path); ok {
		w.Header().Set("Allow", allow)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	s.renderStatus(w, http.StatusNotFound, templateNotFound, s.newTemplateData(r, "Page Not Found"))
}

func postOnlyAllowForPath(path string) (string, bool) {
	switch path {
	case paths.Logout,
		paths.VerifyEmailResend,
		paths.ChangePassword,
		paths.ChangeEmail,
		paths.AccountSessionsRevoke,
		paths.AccountSessionsRevokeOthers:
		return http.MethodPost, true
	default:
		return "", false
	}
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", cspHeaderValue)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if s.secureCookie(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) cacheControlHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSetAuthSensitiveNoStore(r.Method, r.URL.Path) {
			w.Header().Set("Cache-Control", cacheControlNoStore)
			w.Header().Set("Pragma", pragmaNoCache)
			w.Header().Set("Expires", expiresImmediately)
		}
		next.ServeHTTP(w, r)
	})
}

func shouldSetAuthSensitiveNoStore(method, path string) bool {
	switch method {
	case http.MethodGet:
		return isAuthSensitivePagePath(path)
	case http.MethodPost:
		return isAuthSensitivePostPath(path)
	default:
		return false
	}
}

func isAuthSensitivePagePath(path string) bool {
	if path == paths.Login || path == paths.Register {
		return true
	}
	return path == paths.Account || strings.HasPrefix(path, paths.Account+"/")
}

func isAuthSensitivePostPath(path string) bool {
	if path == paths.Login || path == paths.Register || path == paths.Logout {
		return true
	}
	return path == paths.Account || strings.HasPrefix(path, paths.Account+"/")
}

func (s *Server) limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}
