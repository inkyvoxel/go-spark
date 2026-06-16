package app

import (
	"crypto/hmac"
	"crypto/sha256"
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

// deriveKey produces a purpose-scoped key from a root secret using HMAC-SHA256,
// following the single-root-secret pattern used throughout this application.
func deriveKey(base []byte, purpose string) []byte {
	h := hmac.New(sha256.New, base)
	h.Write([]byte(purpose))
	return h.Sum(nil)
}

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
	// TOTP secrets and backup codes are protected at rest under a dedicated root
	// key, separate from SECRET_KEY_BASE. This keeps signing-key rotation (CSRF,
	// flash, ceremony cookies) cheap — it no longer invalidates anyone's second
	// factor. Rotating AUTH_TOTP_KEY, by contrast, forces 2FA re-enrolment.
	totpKey := []byte(strings.TrimSpace(cfg.TOTPKey))

	authStore, err := database.NewAuthStore(db, deriveKey(totpKey, "totp_secret"))
	if err != nil {
		return Runtime{}, fmt.Errorf("configure auth store: %w", err)
	}

	auth, err := services.NewAuthService(authStore, services.AuthOptions{
		PasswordMinLen:           cfg.PasswordMinLength,
		PasswordPepper:           cfg.PasswordPepper,
		TOTPIssuer:               cfg.TOTPIssuer,
		TOTPBackupCodeKey:        deriveKey(totpKey, "totp_backup_code"),
		EmailChangeNoticeEnabled: boolPtr(cfg.EmailChangeNoticeEnabled),
		WebAuthnRPID:             passkeyRPID(cfg),
		WebAuthnRPDisplayName:    passkeyRPDisplayName(cfg),
		WebAuthnRPOrigins:        []string{cfg.AppBaseURL},
		ConfirmationEmail: email.AccountConfirmationOptions{
			AppBaseURL: cfg.AppBaseURL,
			From:       cfg.EmailFrom,
		},
		PasswordResetEmail: email.PasswordResetOptions{
			AppBaseURL: cfg.AppBaseURL,
			From:       cfg.EmailFrom,
		},
	})
	if err != nil {
		return Runtime{}, fmt.Errorf("configure auth service: %w", err)
	}

	backgroundJobs, err := buildJobs(cfg, logger, db)
	if err != nil {
		return Runtime{}, err
	}
	jobsRunner, err := jobs.NewRunner(logger, backgroundJobs...)
	if err != nil {
		return Runtime{}, fmt.Errorf("configure background jobs runner: %w", err)
	}

	webApp, err := server.New(server.Options{
		Logger:            logger,
		DB:                db,
		Auth:              auth,
		CookieSecure:      cfg.CookieSecure,
		AppBaseURL:        cfg.AppBaseURL,
		SecretKeyBase:     secretKeyBase,
		PasswordMinLength: cfg.PasswordMinLength,
		RateLimitPolicies: toServerRateLimitPolicies(cfg.RateLimitPolicies),
		TrustedProxies:    cfg.TrustedProxies,
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

func boolPtr(v bool) *bool {
	return &v
}

const defaultPasskeyRPDisplayName = "Go Spark"

// passkeyRPID resolves the WebAuthn Relying Party ID. It uses the explicit
// override when set, otherwise the host of APP_BASE_URL (without scheme or port).
func passkeyRPID(cfg config.Config) string {
	if strings.TrimSpace(cfg.PasskeyRPID) != "" {
		return strings.TrimSpace(cfg.PasskeyRPID)
	}
	parsed, err := url.Parse(cfg.AppBaseURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func passkeyRPDisplayName(cfg config.Config) string {
	if name := strings.TrimSpace(cfg.PasskeyRPDisplayName); name != "" {
		return name
	}
	return defaultPasskeyRPDisplayName
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
	if strings.TrimSpace(cfg.TOTPKey) == "" {
		return fmt.Errorf("AUTH_TOTP_KEY must be set")
	}
	return nil
}

func logSecurityConfigWarnings(cfg config.Config, logger *slog.Logger) {
	for _, warning := range securityConfigWarnings(cfg) {
		logger.Warn("security configuration warning", "warning", warning)
	}
}

func securityConfigWarnings(cfg config.Config) []string {
	warnings := make([]string, 0, 5)

	if !cfg.CookieSecure {
		warnings = append(warnings, "APP_COOKIE_SECURE=false is not suitable for public production deployments")
	}
	if !isHTTPSURL(cfg.AppBaseURL) {
		warnings = append(warnings, "APP_BASE_URL does not use https; configure https for public production deployments")
	}
	if cfg.EmailProvider != email.ProviderSMTP {
		warnings = append(warnings, fmt.Sprintf("EMAIL_PROVIDER=%q does not deliver real email by default", cfg.EmailProvider))
	}
	if cfg.EmailLogBody {
		warnings = append(warnings, "EMAIL_LOG_BODY=true may expose email contents and token links in logs")
	}
	if isDefaultStarterEmailFrom(cfg.EmailFrom) {
		warnings = append(warnings, "EMAIL_FROM is still the default starter sender")
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
		DeleteAccount: server.RateLimitPolicy{
			MaxRequests: cfg.DeleteAccount.MaxRequests,
			Window:      cfg.DeleteAccount.Window,
		},
		TOTPChallenge: server.RateLimitPolicy{
			MaxRequests: cfg.TOTPChallenge.MaxRequests,
			Window:      cfg.TOTPChallenge.Window,
		},
		TOTPDisable: server.RateLimitPolicy{
			MaxRequests: cfg.TOTPDisable.MaxRequests,
			Window:      cfg.TOTPDisable.Window,
		},
		TOTPConfirm: server.RateLimitPolicy{
			MaxRequests: cfg.TOTPConfirm.MaxRequests,
			Window:      cfg.TOTPConfirm.Window,
		},
		TOTPRegenerateCodes: server.RateLimitPolicy{
			MaxRequests: cfg.TOTPRegenerateCodes.MaxRequests,
			Window:      cfg.TOTPRegenerateCodes.Window,
		},
		PasskeyRegister: server.RateLimitPolicy{
			MaxRequests: cfg.PasskeyRegister.MaxRequests,
			Window:      cfg.PasskeyRegister.Window,
		},
		PasskeyLogin: server.RateLimitPolicy{
			MaxRequests: cfg.PasskeyLogin.MaxRequests,
			Window:      cfg.PasskeyLogin.Window,
		},
		PasskeyManage: server.RateLimitPolicy{
			MaxRequests: cfg.PasskeyManage.MaxRequests,
			Window:      cfg.PasskeyManage.Window,
		},
	}
}
