package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
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

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("application failed", "err", err)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}

	processOverride, err := processArg(args)
	if err != nil {
		return err
	}

	cfg, err := config.FromEnvWithProcess(services.DefaultPasswordMinLength, processOverride)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := validateSecurityConfig(cfg); err != nil {
		return fmt.Errorf("invalid security configuration: %w", err)
	}
	logSecurityConfigWarnings(cfg, logger)
	csrfSigningKey, err := resolveCSRFSigningKey(cfg, logger)
	if err != nil {
		return fmt.Errorf("resolve CSRF signing key: %w", err)
	}

	db, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	auth := services.NewAuthService(database.NewAuthStore(db), services.AuthOptions{
		PasswordMinLen:           cfg.PasswordMinLength,
		PasswordPepper:           cfg.PasswordPepper,
		EmailVerificationPolicy:  services.NewEmailVerificationPolicy(cfg.EmailVerificationRequired),
		EmailChangeNoticeEnabled: boolPtr(cfg.EmailChangeNoticeEnabled),
		ConfirmationEmail: email.AccountConfirmationOptions{
			AppBaseURL: cfg.AppBaseURL,
			From:       authSenderFrom(cfg),
		},
		PasswordResetEmail: email.PasswordResetOptions{
			AppBaseURL: cfg.AppBaseURL,
			From:       authSenderFrom(cfg),
		},
	})

	emailSender, err := newEmailSender(cfg, logger)
	if err != nil {
		return fmt.Errorf("configure email sender: %w", err)
	}

	emailProcessor := email.NewProcessor(database.NewEmailOutboxStore(db), emailSender, email.ProcessorOptions{
		Logger: logger,
	})
	cleanupJob, err := jobs.NewCleanupJob(database.NewCleanupStore(db), jobs.CleanupOptions{
		Logger:               logger,
		TokenRetention:       cfg.CleanupTokenRetention,
		SentEmailRetention:   cfg.CleanupSentEmailRetention,
		FailedEmailRetention: cfg.CleanupFailedEmailRetention,
	})
	if err != nil {
		return fmt.Errorf("configure cleanup job: %w", err)
	}
	jobsRunner, err := jobs.NewRunner(
		logger,
		jobs.NewEmailJob(emailProcessor, jobs.DefaultEmailInterval),
		cleanupJob.Job(cfg.CleanupInterval),
	)
	if err != nil {
		return fmt.Errorf("configure background jobs runner: %w", err)
	}

	app := server.New(server.Options{
		Logger:                  logger,
		DB:                      db,
		Auth:                    auth,
		CookieSecure:            cfg.CookieSecure,
		AppBaseURL:              cfg.AppBaseURL,
		CSRFSigningKey:          csrfSigningKey,
		PasswordMinLength:       cfg.PasswordMinLength,
		EmailVerificationPolicy: services.NewEmailVerificationPolicy(cfg.EmailVerificationRequired),
		RateLimitPolicies:       toServerRateLimitPolicies(cfg.RateLimitPolicies),
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.Process {
	case config.ProcessAll:
		return runAll(ctx, logger, cfg, httpServer, jobsRunner)
	case config.ProcessWeb:
		return runWeb(ctx, logger, cfg, httpServer)
	case config.ProcessWorker:
		return runWorker(ctx, logger, cfg, jobsRunner)
	default:
		return fmt.Errorf("APP_PROCESS must be %q, %q, or %q", config.ProcessAll, config.ProcessWeb, config.ProcessWorker)
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func processArg(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) > 1 {
		return "", fmt.Errorf("expected at most one process mode argument (%q, %q, or %q)", config.ProcessAll, config.ProcessWeb, config.ProcessWorker)
	}

	process := strings.ToLower(strings.TrimSpace(args[0]))
	if !config.IsProcess(process) {
		return "", fmt.Errorf("process mode must be %q, %q, or %q", config.ProcessAll, config.ProcessWeb, config.ProcessWorker)
	}
	return process, nil
}

func runAll(ctx context.Context, logger *slog.Logger, cfg config.Config, httpServer *http.Server, jobsRunner *jobs.Runner) error {
	errs := make(chan error, 2)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
		errs <- httpServer.ListenAndServe()
	}()

	go func() {
		if err := jobsRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		logger.Info("server stopped")
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}

func runWeb(ctx context.Context, logger *slog.Logger, cfg config.Config, httpServer *http.Server) error {
	errs := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
		errs <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		logger.Info("server stopped")
		return nil
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runWorker(ctx context.Context, logger *slog.Logger, cfg config.Config, jobsRunner *jobs.Runner) error {
	logger.Info("background jobs worker starting", "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
	if err := jobsRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("background jobs worker stopped")
	return nil
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
			From:     cfg.SMTPFrom,
			UseTLS:   cfg.SMTPTLS,
		})
	default:
		return nil, fmt.Errorf("unsupported email provider %q", cfg.EmailProvider)
	}
}

func authSenderFrom(cfg config.Config) string {
	if cfg.EmailProvider == email.ProviderSMTP && strings.TrimSpace(cfg.SMTPFrom) != "" {
		return cfg.SMTPFrom
	}
	return cfg.EmailFrom
}

func validateSecurityConfig(cfg config.Config) error {
	if cfg.Env != "production" {
		return nil
	}
	if !cfg.CookieSecure {
		return fmt.Errorf("APP_COOKIE_SECURE must be true when APP_ENV=production")
	}
	if !isHTTPSURL(cfg.AppBaseURL) {
		return fmt.Errorf("APP_BASE_URL must use https when APP_ENV=production")
	}
	if strings.TrimSpace(cfg.PasswordPepper) == "" {
		return fmt.Errorf("AUTH_PASSWORD_PEPPER must be set when APP_ENV=production")
	}
	if strings.TrimSpace(cfg.CSRFSigningKey) == "" {
		return fmt.Errorf("CSRF_SIGNING_KEY must be set when APP_ENV=production")
	}
	return nil
}

func resolveCSRFSigningKey(cfg config.Config, logger *slog.Logger) (string, error) {
	key := strings.TrimSpace(cfg.CSRFSigningKey)
	if key != "" {
		return key, nil
	}

	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	ephemeralKey := base64.RawURLEncoding.EncodeToString(random)
	if logger != nil {
		logger.Warn("CSRF_SIGNING_KEY is not set; generated ephemeral key for non-production process startup")
	}
	return ephemeralKey, nil
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
