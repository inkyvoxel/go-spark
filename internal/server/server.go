package server

import (
	"database/sql"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	maxRequestBodyBytes = 64 * 1024
	cspHeaderValue      = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'self'"
)

type Server struct {
	db                *sql.DB
	auth              authService
	logger            *slog.Logger
	templates         map[string]*template.Template
	cookieSecure      bool
	passwordMinLength int
	rateLimiter       rateLimitStore
	rateLimitPolicies RateLimitPolicies
}

type Options struct {
	DB                *sql.DB
	Auth              authService
	Logger            *slog.Logger
	CookieSecure      bool
	PasswordMinLength int
	RateLimitPolicies RateLimitPolicies
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

	return &Server{
		db:                opts.DB,
		auth:              opts.Auth,
		logger:            logger,
		templates:         mustParseTemplates(),
		cookieSecure:      opts.CookieSecure,
		passwordMinLength: passwordMinLength,
		rateLimiter:       newInMemoryRateLimiter(),
		rateLimitPolicies: rateLimitPoliciesWithDefaults(opts.RateLimitPolicies),
	}
}

func mustParseTemplates() map[string]*template.Template {
	templates, err := parseTemplates()
	if err != nil {
		panic(err)
	}

	return templates
}

func parseTemplates() (map[string]*template.Template, error) {
	pages := []string{
		"account.html",
		"confirm_email.html",
		"forgot_password.html",
		"home.html",
		"login.html",
		"reset_password.html",
		"register.html",
		"resend_verification.html",
		"verify_email.html",
	}
	templates := make(map[string]*template.Template, len(pages))
	layout := filepath.Join("templates", "layout.html")

	for _, page := range pages {
		parsed, err := template.ParseFiles(layout, filepath.Join("templates", page))
		if err != nil {
			return nil, err
		}
		templates[page] = parsed
	}

	return templates, nil
}

func (s *Server) Routes() http.Handler {
	s.ensureRateLimiting()

	mux := http.NewServeMux()
	dynamic := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /healthz", s.health)

	// Register new protected pages with requireAuth and anonymous-only pages with requireAnonymous.
	dynamic.Handle("GET /register", s.requireAnonymous(http.HandlerFunc(s.registerForm)))
	dynamic.Handle(
		"POST /register",
		s.requireAnonymous(
			s.withRateLimit("register", s.rateLimitPolicies.Register, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.register)),
		),
	)
	dynamic.HandleFunc("GET /confirm-email", s.confirmEmail)
	dynamic.Handle("GET /forgot-password", s.requireAnonymous(http.HandlerFunc(s.forgotPasswordForm)))
	dynamic.Handle(
		"POST /forgot-password",
		s.requireAnonymous(
			s.withRateLimit("forgot-password", s.rateLimitPolicies.ForgotPassword, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.forgotPassword)),
		),
	)
	dynamic.Handle("GET /login", s.requireAnonymous(http.HandlerFunc(s.loginForm)))
	dynamic.Handle(
		"POST /login",
		s.requireAnonymous(
			s.withRateLimit("login", s.rateLimitPolicies.Login, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.login)),
		),
	)
	dynamic.Handle("GET /reset-password", s.requireAnonymous(http.HandlerFunc(s.resetPasswordForm)))
	dynamic.Handle("POST /reset-password", s.requireAnonymous(http.HandlerFunc(s.resetPassword)))
	dynamic.Handle("GET /resend-verification", s.requireAnonymous(http.HandlerFunc(s.resendVerificationForm)))
	dynamic.Handle(
		"POST /resend-verification",
		s.requireAnonymous(
			s.withRateLimit("resend-verification-public", s.rateLimitPolicies.PublicResendVerification, rateLimitKeyByIPAndEmail("email"), http.HandlerFunc(s.resendVerificationPublic)),
		),
	)
	dynamic.Handle("POST /logout", s.requireAuth(http.HandlerFunc(s.logout)))
	dynamic.Handle("GET /verify-email", s.requireAuth(http.HandlerFunc(s.verifyEmail)))
	dynamic.Handle("GET /account", s.requireVerifiedAuth(http.HandlerFunc(s.account)))
	dynamic.Handle(
		"POST /account/resend-verification",
		s.requireAuth(
			s.withRateLimit("resend-verification-account", s.rateLimitPolicies.AccountResendVerification, rateLimitKeyByIPAndUser(), http.HandlerFunc(s.resendVerification)),
		),
	)
	dynamic.Handle("POST /account/change-password", s.requireVerifiedAuth(http.HandlerFunc(s.changePassword)))
	dynamic.HandleFunc("GET /", s.home)

	mux.Handle("/", s.securityHeaders(s.limitRequestBody(s.csrf(s.loadSession(dynamic)))))

	return s.logRequests(mux)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, "home.html", s.newTemplateData(r, "Go Spark"))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		s.logger.Error("health check failed", "err", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.logger.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
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

func (s *Server) limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}
