package server

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	appassets "github.com/inkyvoxel/go-spark"
	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/ratelimit"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	maxRequestBodyBytes = 64 * 1024
	cspHeaderValue      = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'self'"
	cacheControlNoStore = "no-store"
	cacheControlPublic  = "public, max-age=86400"
	pragmaNoCache       = "no-cache"
	expiresImmediately  = "0"
)

type Server struct {
	db                *sql.DB
	auth              authService
	projects          projectService // example feature; remove with the projects example
	logger            *slog.Logger
	templates         map[string]*template.Template
	cookieSecure      bool
	crossOrigin       *http.CrossOriginProtection
	passwordMinLength int
	cookieSigningKey  []byte
	flashKey          []byte
	rateLimiter       *fixedWindowRateLimiter
	rateLimitPolicies RateLimitPolicies
	trustedProxies    []net.IPNet
	postOnlyPaths     map[string]struct{}
}

type Options struct {
	DB                *sql.DB
	Auth              authService
	Projects          projectService // example feature; remove with the projects example
	Logger            *slog.Logger
	CookieSecure      bool
	AppBaseURL        string
	SecretKeyBase     string
	PasswordMinLength int
	RateLimitPolicies RateLimitPolicies
	TrustedProxies    []string
}

func New(opts Options) (*Server, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Auth == nil {
		return nil, fmt.Errorf("server auth service is required")
	}

	passwordMinLength := opts.PasswordMinLength
	if passwordMinLength == 0 {
		passwordMinLength = services.DefaultPasswordMinLength
	}
	secretKeyBase := []byte(strings.TrimSpace(opts.SecretKeyBase))
	cookieSigningKey := services.DeriveKey(secretKeyBase, "cookie-signing")
	flashKey := services.DeriveKey(secretKeyBase, "flash")
	crossOrigin, err := newCrossOriginProtection(normalizeOrigin(opts.AppBaseURL))
	if err != nil {
		return nil, fmt.Errorf("configure cross-origin protection: %w", err)
	}

	templates, err := parseTemplates()
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	trustedProxies, err := parseTrustedProxies(opts.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("parse trusted proxies: %w", err)
	}

	return &Server{
		db:                opts.DB,
		auth:              opts.Auth,
		projects:          opts.Projects,
		logger:            logger,
		templates:         templates,
		cookieSecure:      opts.CookieSecure,
		crossOrigin:       crossOrigin,
		passwordMinLength: passwordMinLength,
		cookieSigningKey:  cookieSigningKey,
		flashKey:          flashKey,
		rateLimiter:       newFixedWindowRateLimiter(),
		rateLimitPolicies: ratelimit.Resolve(opts.RateLimitPolicies),
		trustedProxies:    trustedProxies,
		postOnlyPaths:     make(map[string]struct{}),
	}, nil
}

func parseTrustedProxies(raw []string) ([]net.IPNet, error) {
	result := make([]net.IPNet, 0, len(raw))
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", entry, err)
			}
			result = append(result, *cidr)
		} else {
			ip := net.ParseIP(entry)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy IP %q", entry)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			_, cidr, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), bits))
			result = append(result, *cidr)
		}
	}
	return result, nil
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

func parseTemplates() (map[string]*template.Template, error) {
	pages := map[string]string{
		templateNotFound:             "404.html",
		templateInternalError:        "500.html",
		templateHome:                 "home.html",
		templateAccount:              path.Join("account", "account.html"),
		templateChangePassword:       path.Join("account", "change_password.html"),
		templateLogin:                path.Join("account", "login.html"),
		templateRegister:             path.Join("account", "register.html"),
		templateForgotPassword:       path.Join("account", "forgot_password.html"),
		templateResetPassword:        path.Join("account", "reset_password.html"),
		templateConfirmEmail:         path.Join("account", "confirm_email.html"),
		templateResendVerification:   path.Join("account", "resend_verification.html"),
		templateVerifyEmail:          path.Join("account", "verify_email.html"),
		templateChangeEmail:          path.Join("account", "change_email.html"),
		templateConfirmEmailChange:   path.Join("account", "confirm_email_change.html"),
		templateDeleteAccount:        path.Join("account", "delete_account.html"),
		templateTwoFactor:            path.Join("account", "two_factor.html"),
		templateTwoFactorBackupCodes: path.Join("account", "two_factor_backup_codes.html"),
		templateTwoFactorChallenge:   path.Join("account", "two_factor_challenge.html"),
		templatePasskeys:             path.Join("account", "passkeys.html"),
		templateProjects:             path.Join("projects", "index.html"),
	}
	funcMap := template.FuncMap{
		"urlQueryEscape": func(s string) template.URL {
			return template.URL(url.QueryEscape(s))
		},
	}
	templates := make(map[string]*template.Template, len(pages))
	layout := path.Join("templates", templateLayout)
	partials := []string{
		path.Join("templates", templateBreadcrumb),
		path.Join("templates", templateFlash),
	}

	for name, filePath := range pages {
		files := append([]string{layout}, partials...)
		files = append(files, path.Join("templates", filePath))
		parsed, err := template.New(templateLayout).Funcs(funcMap).ParseFS(appassets.FS, files...)
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
	mux.HandleFunc(route(http.MethodGet, paths.RobotsTxt), s.robotsTxt)
	mux.Handle(route(http.MethodGet, paths.StaticPrefix), staticFileHandler())

	s.registerAuthRoutes(dynamic)
	s.registerProjectRoutes(dynamic) // example feature; remove with the projects example
	dynamic.HandleFunc(route(http.MethodGet, "/{$}"), s.home)
	dynamic.HandleFunc(route(http.MethodGet, "/{path...}"), s.notFoundPage)

	handler := s.loadSession(http.Handler(dynamic))
	handler = s.csrf(handler)
	mux.Handle(paths.Home, s.cacheControlHeaders(s.limitRequestBody(handler)))

	// securityHeaders wraps the whole mux so static assets and the health and
	// robots endpoints get the baseline headers (notably X-Content-Type-Options:
	// nosniff) too, not just the dynamic application routes.
	return s.withRequestID(s.logRequests(s.securityHeaders(mux)))
}

func (s *Server) registerAuthRoutes(dynamic *http.ServeMux) {
	dynamic.Handle(route(http.MethodGet, paths.Register), s.requireAnonymous(http.HandlerFunc(s.registerForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.Register),
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.register),
				rateLimit("register-ip", s.rateLimitPolicies[ratelimit.RegisterPerIP], s.rateLimitKeyByIP()),
				rateLimit("register", s.rateLimitPolicies[ratelimit.Register], s.rateLimitKeyByIPAndEmail("email")),
			),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.Login), s.requireAnonymous(http.HandlerFunc(s.loginForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.Login),
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.login),
				rateLimit("login-ip", s.rateLimitPolicies[ratelimit.LoginPerIP], s.rateLimitKeyByIP()),
				rateLimit("login", s.rateLimitPolicies[ratelimit.Login], s.rateLimitKeyByIPAndEmail("email")),
			),
		),
	)
	s.postOnly(dynamic, paths.Logout, s.requireAuth(http.HandlerFunc(s.logout)))
	dynamic.Handle(route(http.MethodGet, paths.Account), s.requireVerifiedAuth(http.HandlerFunc(s.account)))
	dynamic.Handle(route(http.MethodGet, paths.ChangePassword), s.requireVerifiedAuth(http.HandlerFunc(s.changePasswordForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ChangePassword),
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.changePassword), rateLimit("change-password", s.rateLimitPolicies[ratelimit.ChangePassword], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.AccountSessionsRevoke,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.revokeSession), rateLimit("revoke-session", s.rateLimitPolicies[ratelimit.RevokeSession], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.AccountSessionsRevokeOthers,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.revokeOtherSessions), rateLimit("revoke-other-sessions", s.rateLimitPolicies[ratelimit.RevokeOtherSessions], s.rateLimitKeyByIPAndUser())),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.ForgotPassword), s.requireAnonymous(http.HandlerFunc(s.forgotPasswordForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ForgotPassword),
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.forgotPassword),
				rateLimit("forgot-password-ip", s.rateLimitPolicies[ratelimit.ForgotPasswordPerIP], s.rateLimitKeyByIP()),
				rateLimit("forgot-password", s.rateLimitPolicies[ratelimit.ForgotPassword], s.rateLimitKeyByIPAndEmail("email")),
			),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.ResetPassword), s.requireAnonymous(http.HandlerFunc(s.resetPasswordForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ResetPassword),
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.resetPassword), rateLimit("reset-password", s.rateLimitPolicies[ratelimit.ResetPassword], s.rateLimitKeyByIPAndResetTokenCookie())),
		),
	)
	dynamic.HandleFunc(route(http.MethodGet, paths.ConfirmEmail), s.confirmEmail)
	dynamic.Handle(route(http.MethodGet, paths.ResendVerification), s.requireAnonymous(http.HandlerFunc(s.resendVerificationForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ResendVerification),
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.resendVerificationPublic),
				rateLimit("resend-verification-public-ip", s.rateLimitPolicies[ratelimit.PublicResendVerifyPerIP], s.rateLimitKeyByIP()),
				rateLimit("resend-verification-public", s.rateLimitPolicies[ratelimit.PublicResendVerification], s.rateLimitKeyByIPAndEmail("email")),
			),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.VerifyEmail), s.requireAuth(http.HandlerFunc(s.verifyEmail)))
	s.postOnly(dynamic, paths.VerifyEmailResend,
		s.requireAuth(
			s.withRateLimits(http.HandlerFunc(s.resendVerification), rateLimit("resend-verification-account", s.rateLimitPolicies[ratelimit.AccountResendVerification], s.rateLimitKeyByIPAndUser())),
		),
	)
	dynamic.HandleFunc(route(http.MethodGet, paths.ConfirmEmailChange), s.confirmEmailChange)
	dynamic.Handle(route(http.MethodGet, paths.ChangeEmail), s.requireVerifiedAuth(http.HandlerFunc(s.changeEmailForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.ChangeEmail),
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.changeEmail), rateLimit("change-email", s.rateLimitPolicies[ratelimit.ChangeEmail], s.rateLimitKeyByIPAndUser())),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.AccountDelete), s.requireVerifiedAuth(http.HandlerFunc(s.deleteAccountForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.AccountDelete),
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.deleteAccount), rateLimit("delete-account", s.rateLimitPolicies[ratelimit.DeleteAccount], s.rateLimitKeyByIPAndUser())),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.AccountTwoFactor), s.requireVerifiedAuth(http.HandlerFunc(s.twoFactorPage)))
	s.postOnly(dynamic, paths.AccountTwoFactorSetup,
		s.requireVerifiedAuth(http.HandlerFunc(s.twoFactorSetup)),
	)
	s.postOnly(dynamic, paths.AccountTwoFactorConfirm,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.twoFactorConfirm), rateLimit("totp-confirm", s.rateLimitPolicies[ratelimit.TOTPConfirm], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.AccountTwoFactorDisable,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.twoFactorDisable), rateLimit("totp-disable", s.rateLimitPolicies[ratelimit.TOTPDisable], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.AccountTwoFactorRegenerateCodes,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.twoFactorRegenerateCodes), rateLimit("totp-regenerate-codes", s.rateLimitPolicies[ratelimit.TOTPRegenerateCodes], s.rateLimitKeyByIPAndUser())),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.AccountTwoFactorChallenge), s.requireAnonymous(http.HandlerFunc(s.twoFactorChallengeForm)))
	dynamic.Handle(
		route(http.MethodPost, paths.AccountTwoFactorChallenge),
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.twoFactorChallenge), rateLimit("totp-challenge", s.rateLimitPolicies[ratelimit.TOTPChallenge], s.rateLimitKeyByIP())),
		),
	)
	dynamic.Handle(route(http.MethodGet, paths.AccountPasskeys), s.requireVerifiedAuth(http.HandlerFunc(s.passkeysPage)))
	s.postOnly(dynamic, paths.AccountPasskeysRegisterBegin,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.passkeyRegisterBegin), rateLimit("passkey-register-begin", s.rateLimitPolicies[ratelimit.PasskeyRegister], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.AccountPasskeysRegisterFinish,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.passkeyRegisterFinish), rateLimit("passkey-register-finish", s.rateLimitPolicies[ratelimit.PasskeyRegister], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.AccountPasskeysRename,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.passkeyRename), rateLimit("passkey-rename", s.rateLimitPolicies[ratelimit.PasskeyManage], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.AccountPasskeysDelete,
		s.requireVerifiedAuth(
			s.withRateLimits(http.HandlerFunc(s.passkeyDelete), rateLimit("passkey-delete", s.rateLimitPolicies[ratelimit.PasskeyManage], s.rateLimitKeyByIPAndUser())),
		),
	)
	s.postOnly(dynamic, paths.LoginPasskeyBegin,
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.passkeyLoginBegin), rateLimit("passkey-login-begin", s.rateLimitPolicies[ratelimit.PasskeyLogin], s.rateLimitKeyByIP())),
		),
	)
	s.postOnly(dynamic, paths.LoginPasskeyFinish,
		s.requireAnonymous(
			s.withRateLimits(http.HandlerFunc(s.passkeyLoginFinish), rateLimit("passkey-login-finish", s.rateLimitPolicies[ratelimit.PasskeyLogin], s.rateLimitKeyByIP())),
		),
	)
}

func route(method, path string) string {
	return method + " " + path
}

// postOnly registers a POST-only route and records the path so the catch-all
// GET handler can return 405 instead of 404. Use this instead of mux.Handle
// whenever a route has no corresponding GET handler.
func (s *Server) postOnly(mux *http.ServeMux, path string, handler http.Handler) {
	s.postOnlyPaths[path] = struct{}{}
	mux.Handle(route(http.MethodPost, path), handler)
}

func staticFileHandler() http.Handler {
	fileServer := http.StripPrefix(paths.StaticPrefix, http.FileServerFS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent directory listing from exposing static tree contents.
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(&staticCacheHeaderResponseWriter{ResponseWriter: w}, r)
	})
}

type staticCacheHeaderResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *staticCacheHeaderResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if statusCode == http.StatusOK || statusCode == http.StatusNotModified {
		w.Header().Set("Cache-Control", cacheControlPublic)
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *staticCacheHeaderResponseWriter) Write(data []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.ResponseWriter.Write(data)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, templateHome, s.newTemplateData(w, r, "Home"))
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

const robotsTxtContent = "User-agent: *\n" +
	"Disallow: /login\n" +
	"Disallow: /register\n" +
	"Disallow: /logout\n" +
	"Disallow: /account/\n" +
	"Disallow: /healthz\n" +
	"Disallow: /readyz\n"

func (s *Server) robotsTxt(w http.ResponseWriter, _ *http.Request) {
	writePlaintext(w, http.StatusOK, robotsTxtContent)
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

func (s *Server) internalServerError(w http.ResponseWriter, r *http.Request) {
	s.renderStatus(w, http.StatusInternalServerError, templateInternalError, s.newTemplateData(w, r, "Something went wrong"))
}

func (s *Server) notFoundPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.postOnlyPaths[r.URL.Path]; ok {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	s.renderStatus(w, http.StatusNotFound, templateNotFound, s.newTemplateData(w, r, "Page Not Found"))
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", cspHeaderValue)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
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
		if isUnsafeMethod(r.Method) {
			// Reject declared-oversize bodies up front so the limit holds
			// regardless of which downstream reader touches the body first
			// (e.g. the rate limiter parses the form before the handler does).
			if r.ContentLength > maxRequestBodyBytes {
				http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
			}
		}
		next.ServeHTTP(w, r)
	})
}
