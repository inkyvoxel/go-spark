package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/inkyvoxel/go-spark/internal/config"
	"github.com/inkyvoxel/go-spark/internal/database"
	"github.com/inkyvoxel/go-spark/internal/email"
	"github.com/inkyvoxel/go-spark/internal/server"
	"github.com/inkyvoxel/go-spark/internal/services"
)

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

	db, err := database.Open(cfg.DatabasePath)
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

	emailWorker := email.NewWorker(database.NewEmailOutboxStore(db), emailSender, email.WorkerOptions{
		Logger: logger,
	})

	app := server.New(server.Options{
		Logger:                  logger,
		DB:                      db,
		Auth:                    auth,
		CookieSecure:            cfg.CookieSecure,
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
		return runAll(ctx, logger, cfg, httpServer, emailWorker)
	case config.ProcessWeb:
		return runWeb(ctx, logger, cfg, httpServer)
	case config.ProcessWorker:
		return runWorker(ctx, logger, cfg, emailWorker)
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

func runAll(ctx context.Context, logger *slog.Logger, cfg config.Config, httpServer *http.Server, emailWorker *email.Worker) error {
	errs := make(chan error, 2)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
		errs <- httpServer.ListenAndServe()
	}()

	go func() {
		if err := emailWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
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

func runWorker(ctx context.Context, logger *slog.Logger, cfg config.Config, emailWorker *email.Worker) error {
	logger.Info("email worker starting", "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
	if err := emailWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("email worker stopped")
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
	if strings.TrimSpace(cfg.PasswordPepper) == "" {
		return fmt.Errorf("AUTH_PASSWORD_PEPPER must be set when APP_ENV=production")
	}
	return nil
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
	}
}
