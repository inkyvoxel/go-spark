package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/inkyvoxel/go-spark/internal/config"
	"github.com/inkyvoxel/go-spark/internal/database"
	"github.com/inkyvoxel/go-spark/internal/email"
	"github.com/inkyvoxel/go-spark/internal/jobs"
	"github.com/inkyvoxel/go-spark/internal/platform/sqlite"
	"github.com/inkyvoxel/go-spark/internal/server"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const defaultStarterEmailFrom = `"Go Spark" <hello@example.com>`

type Runtime struct {
	DB         *sql.DB
	HTTPServer *http.Server
	JobsRunner *jobs.Runner
}

func (r Runtime) Close() error {
	if r.DB == nil {
		return nil
	}
	return r.DB.Close()
}

func Build(cfg config.Config, logger *slog.Logger) (Runtime, error) {
	if err := validateSecurityConfig(cfg); err != nil {
		return Runtime{}, fmt.Errorf("invalid security configuration: %w", err)
	}
	logSecurityConfigWarnings(cfg, logger)

	secretKeyBase := strings.TrimSpace(cfg.SecretKeyBase)

	db, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return Runtime{}, fmt.Errorf("open database: %w", err)
	}

	runtime, err := buildRuntime(cfg, logger, db, secretKeyBase)
	if err != nil {
		db.Close()
		return Runtime{}, err
	}

	return runtime, nil
}

func buildRuntime(cfg config.Config, logger *slog.Logger, db *sql.DB, secretKeyBase string) (Runtime, error) {
	auth := services.NewAuthService(database.NewAuthStore(db), services.AuthOptions{
		PasswordMinLen:           cfg.PasswordMinLength,
		PasswordPepper:           cfg.PasswordPepper,
		EmailVerificationPolicy:  services.NewEmailVerificationPolicy(cfg.EmailVerificationRequired),
		EmailChangeNoticeEnabled: boolPtr(cfg.EmailChangeNoticeEnabled),
		ConfirmationEmail: email.AccountConfirmationOptions{
			AppBaseURL: cfg.AppBaseURL,
			From:       cfg.EmailFrom,
		},
		PasswordResetEmail: email.PasswordResetOptions{
			AppBaseURL: cfg.AppBaseURL,
			From:       cfg.EmailFrom,
		},
	})

	backgroundJobs, err := buildJobs(cfg, logger, db)
	if err != nil {
		return Runtime{}, err
	}
	jobsRunner, err := jobs.NewRunner(logger, backgroundJobs...)
	if err != nil {
		return Runtime{}, fmt.Errorf("configure background jobs runner: %w", err)
	}

	webApp, err := server.New(server.Options{
		Logger:                  logger,
		DB:                      db,
		Auth:                    auth,
		CookieSecure:            cfg.CookieSecure,
		AppBaseURL:              cfg.AppBaseURL,
		SecretKeyBase:           secretKeyBase,
		PasswordMinLength:       cfg.PasswordMinLength,
		EmailVerificationPolicy: services.NewEmailVerificationPolicy(cfg.EmailVerificationRequired),
		RateLimitPolicies:       toServerRateLimitPolicies(cfg.RateLimitPolicies),
		TrustedProxies:          cfg.TrustedProxies,
	})
	if err != nil {
		return Runtime{}, fmt.Errorf("configure web server: %w", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           webApp.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return Runtime{
		DB:         db,
		HTTPServer: httpServer,
		JobsRunner: jobsRunner,
	}, nil
}

type serverAuthService = interface {
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
}

func boolPtr(v bool) *bool {
	return &v
}

func buildJobs(cfg config.Config, logger *slog.Logger, db *sql.DB) ([]jobs.Job, error) {
	configured := make([]jobs.Job, 0, 2)

	emailSender, err := newEmailSender(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("configure email sender: %w", err)
	}
	emailProcessor := email.NewProcessor(database.NewEmailOutboxStore(db), emailSender, email.ProcessorOptions{
		Logger: logger,
	})
	configured = append(configured, jobs.NewEmailJob(emailProcessor, jobs.DefaultEmailInterval))

	cleanupJob, err := jobs.NewCleanupJob(database.NewCleanupStore(db), jobs.CleanupOptions{
		Logger:               logger,
		TokenRetention:       cfg.CleanupTokenRetention,
		SentEmailRetention:   cfg.CleanupSentEmailRetention,
		FailedEmailRetention: cfg.CleanupFailedEmailRetention,
	})
	if err != nil {
		return nil, fmt.Errorf("configure cleanup job: %w", err)
	}
	configured = append(configured, cleanupJob.Job(cfg.CleanupInterval))

	return configured, nil
}

func newEmailSender(cfg config.Config, logger *slog.Logger) (email.Sender, error) {
	switch cfg.EmailProvider {
	case email.ProviderLog:
		return email.NewLogSender(logger, email.LogSenderOptions{
			LogBody: cfg.EmailLogBody,
		}), nil
	case email.ProviderSMTP:
		return email.NewSMTPSender(email.SMTPSenderOptions{
			Logger:   logger,
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.EmailFrom,
			UseTLS:   cfg.SMTPTLS,
		})
	default:
		return nil, fmt.Errorf("unsupported email provider %q", cfg.EmailProvider)
	}
}

func validateSecurityConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.PasswordPepper) == "" {
		return fmt.Errorf("AUTH_PASSWORD_PEPPER must be set")
	}
	if strings.TrimSpace(cfg.SecretKeyBase) == "" {
		return fmt.Errorf("SECRET_KEY_BASE must be set")
	}
	if cfg.Env != "production" {
		return nil
	}
	if !cfg.CookieSecure {
		return fmt.Errorf("APP_COOKIE_SECURE must be true when APP_ENV=production")
	}
	if !isHTTPSURL(cfg.AppBaseURL) {
		return fmt.Errorf("APP_BASE_URL must use https when APP_ENV=production")
	}
	return nil
}

func logSecurityConfigWarnings(cfg config.Config, logger *slog.Logger) {
	for _, warning := range securityConfigWarnings(cfg) {
		logger.Warn("production security configuration warning", "warning", warning)
	}
}

func securityConfigWarnings(cfg config.Config) []string {
	if cfg.Env != "production" {
		return nil
	}

	warnings := make([]string, 0, 4)

	if !cfg.EmailVerificationRequired {
		warnings = append(warnings, "AUTH_EMAIL_VERIFICATION_REQUIRED=false allows unverified users to access account features in production")
	}
	if cfg.EmailProvider != email.ProviderSMTP {
		warnings = append(warnings, fmt.Sprintf("EMAIL_PROVIDER=%q in production does not deliver real email by default", cfg.EmailProvider))
	}
	if cfg.EmailLogBody {
		warnings = append(warnings, "EMAIL_LOG_BODY=true may expose email contents and token links in production logs")
	}
	if isDefaultStarterEmailFrom(cfg.EmailFrom) {
		warnings = append(warnings, "EMAIL_FROM is still the default starter sender in production")
	}

	return warnings
}

func isHTTPSURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

func isDefaultStarterEmailFrom(value string) bool {
	return strings.TrimSpace(value) == defaultStarterEmailFrom
}

func toServerRateLimitPolicies(cfg config.RateLimitPoliciesConfig) server.RateLimitPolicies {
	return server.RateLimitPolicies{
		Login: server.RateLimitPolicy{
			MaxRequests: cfg.Login.MaxRequests,
			Window:      cfg.Login.Window,
		},
		Register: server.RateLimitPolicy{
			MaxRequests: cfg.Register.MaxRequests,
			Window:      cfg.Register.Window,
		},
		ForgotPassword: server.RateLimitPolicy{
			MaxRequests: cfg.ForgotPassword.MaxRequests,
			Window:      cfg.ForgotPassword.Window,
		},
		ResetPassword: server.RateLimitPolicy{
			MaxRequests: cfg.ResetPassword.MaxRequests,
			Window:      cfg.ResetPassword.Window,
		},
		PublicResendVerification: server.RateLimitPolicy{
			MaxRequests: cfg.PublicResendVerification.MaxRequests,
			Window:      cfg.PublicResendVerification.Window,
		},
		AccountResendVerification: server.RateLimitPolicy{
			MaxRequests: cfg.AccountResendVerification.MaxRequests,
			Window:      cfg.AccountResendVerification.Window,
		},
		ChangePassword: server.RateLimitPolicy{
			MaxRequests: cfg.ChangePassword.MaxRequests,
			Window:      cfg.ChangePassword.Window,
		},
		ChangeEmail: server.RateLimitPolicy{
			MaxRequests: cfg.ChangeEmail.MaxRequests,
			Window:      cfg.ChangeEmail.Window,
		},
		RevokeSession: server.RateLimitPolicy{
			MaxRequests: cfg.RevokeSession.MaxRequests,
			Window:      cfg.RevokeSession.Window,
		},
		RevokeOtherSessions: server.RateLimitPolicy{
			MaxRequests: cfg.RevokeOtherSessions.MaxRequests,
			Window:      cfg.RevokeOtherSessions.Window,
		},
	}
}
