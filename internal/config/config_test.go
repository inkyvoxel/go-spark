package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inkyvoxel/go-spark/internal/services"
)

func TestLoadDotEnvLoadsValuesWithoutOverridingEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "shell")
	unsetEnvForTest(t, "TEST_DOTENV_ONLY_VALUE")

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("APP_ENV=dotenv\nTEST_DOTENV_ONLY_VALUE=loaded\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv() error = %v", err)
	}

	if got := os.Getenv("APP_ENV"); got != "shell" {
		t.Fatalf("APP_ENV = %q, want existing shell value", got)
	}
	if got := os.Getenv("TEST_DOTENV_ONLY_VALUE"); got != "loaded" {
		t.Fatalf("TEST_DOTENV_ONLY_VALUE = %q, want dotenv value", got)
	}
}

func TestLoadDotEnvIgnoresMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv() error = %v, want nil", err)
	}
}

func TestLoadDotEnvReturnsParseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(`APP_ENV="unterminated`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := LoadDotEnv(path)
	if err == nil {
		t.Fatal("LoadDotEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "load") {
		t.Fatalf("LoadDotEnv() error = %v, want load context", err)
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()

	original, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}

	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, original)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func TestFromEnvUsesDefaults(t *testing.T) {
	t.Setenv("APP_ADDR", "")
	t.Setenv("APP_PROCESS", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("APP_COOKIE_SECURE", "")
	t.Setenv("SECRET_KEY_BASE", "")
	t.Setenv("AUTH_PASSWORD_MIN_LENGTH", "")
	t.Setenv("AUTH_PASSWORD_PEPPER", "")
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("AUTH_EMAIL_VERIFICATION_REQUIRED", "")
	t.Setenv("AUTH_EMAIL_CHANGE_NOTICE_ENABLED", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("EMAIL_PROVIDER", "")
	t.Setenv("EMAIL_LOG_BODY", "")
	t.Setenv("JOBS_CLEANUP_INTERVAL", "")
	t.Setenv("JOBS_CLEANUP_TOKEN_RETENTION", "")
	t.Setenv("JOBS_CLEANUP_SENT_EMAIL_RETENTION", "")
	t.Setenv("JOBS_CLEANUP_FAILED_EMAIL_RETENTION", "")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.Process != ProcessAll {
		t.Fatalf("Process = %q, want %q", cfg.Process, ProcessAll)
	}
	if cfg.Env != "development" {
		t.Fatalf("Env = %q, want %q", cfg.Env, "development")
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
	if cfg.DatabasePath != "./data/app.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "./data/app.db")
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure = true, want false")
	}
	if cfg.SecretKeyBase != "" {
		t.Fatalf("SecretKeyBase = %q, want empty", cfg.SecretKeyBase)
	}
	if !cfg.EmailVerificationRequired {
		t.Fatal("EmailVerificationRequired = false, want true")
	}
	if !cfg.EmailChangeNoticeEnabled {
		t.Fatal("EmailChangeNoticeEnabled = false, want true")
	}
	if cfg.PasswordMinLength != services.DefaultPasswordMinLength {
		t.Fatalf("PasswordMinLength = %d, want %d", cfg.PasswordMinLength, services.DefaultPasswordMinLength)
	}
	if cfg.PasswordPepper != "" {
		t.Fatalf("PasswordPepper = %q, want empty", cfg.PasswordPepper)
	}
	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Fatalf("AppBaseURL = %q, want %q", cfg.AppBaseURL, "http://localhost:8080")
	}
	if cfg.EmailFrom != `"Go Spark" <hello@example.com>` {
		t.Fatalf("EmailFrom = %q, want formatted sender", cfg.EmailFrom)
	}
	if cfg.EmailProvider != EmailProviderLog {
		t.Fatalf("EmailProvider = %q, want %q", cfg.EmailProvider, EmailProviderLog)
	}
	if cfg.EmailLogBody {
		t.Fatal("EmailLogBody = true, want false")
	}
	if cfg.CleanupInterval != time.Hour {
		t.Fatalf("CleanupInterval = %v, want %v", cfg.CleanupInterval, time.Hour)
	}
	if cfg.CleanupTokenRetention != 24*time.Hour {
		t.Fatalf("CleanupTokenRetention = %v, want %v", cfg.CleanupTokenRetention, 24*time.Hour)
	}
	if cfg.CleanupSentEmailRetention != 7*24*time.Hour {
		t.Fatalf("CleanupSentEmailRetention = %v, want %v", cfg.CleanupSentEmailRetention, 7*24*time.Hour)
	}
	if cfg.CleanupFailedEmailRetention != 14*24*time.Hour {
		t.Fatalf("CleanupFailedEmailRetention = %v, want %v", cfg.CleanupFailedEmailRetention, 14*24*time.Hour)
	}
}

func TestFromEnvUsesEnvironment(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("APP_PROCESS", "WORKER")
	t.Setenv("APP_ENV", "test")
	t.Setenv("LOG_FORMAT", "JSON")
	t.Setenv("DATABASE_PATH", "/tmp/app-test.db")
	t.Setenv("APP_COOKIE_SECURE", "true")
	t.Setenv("SECRET_KEY_BASE", "csrf-signing-key")
	t.Setenv("AUTH_PASSWORD_MIN_LENGTH", "16")
	t.Setenv("APP_BASE_URL", "https://app.example.com/")
	t.Setenv("AUTH_EMAIL_VERIFICATION_REQUIRED", "false")
	t.Setenv("AUTH_EMAIL_CHANGE_NOTICE_ENABLED", "false")
	t.Setenv("AUTH_PASSWORD_PEPPER", "super-secret-pepper")
	t.Setenv("EMAIL_FROM", "Example <mail@example.com>")
	t.Setenv("EMAIL_PROVIDER", "LOG")
	t.Setenv("EMAIL_LOG_BODY", "true")
	t.Setenv("JOBS_CLEANUP_INTERVAL", "30m")
	t.Setenv("JOBS_CLEANUP_TOKEN_RETENTION", "36h")
	t.Setenv("JOBS_CLEANUP_SENT_EMAIL_RETENTION", "240h")
	t.Setenv("JOBS_CLEANUP_FAILED_EMAIL_RETENTION", "360h")
	t.Setenv("RATE_LIMIT_LOGIN_MAX_REQUESTS", "7")
	t.Setenv("RATE_LIMIT_LOGIN_WINDOW", "2m")
	t.Setenv("RATE_LIMIT_RESET_PASSWORD_MAX_REQUESTS", "8")
	t.Setenv("RATE_LIMIT_RESET_PASSWORD_WINDOW", "30m")
	t.Setenv("RATE_LIMIT_CHANGE_PASSWORD_MAX_REQUESTS", "4")
	t.Setenv("RATE_LIMIT_CHANGE_PASSWORD_WINDOW", "20m")
	t.Setenv("RATE_LIMIT_CHANGE_EMAIL_MAX_REQUESTS", "6")
	t.Setenv("RATE_LIMIT_CHANGE_EMAIL_WINDOW", "25m")
	t.Setenv("RATE_LIMIT_REVOKE_SESSION_MAX_REQUESTS", "9")
	t.Setenv("RATE_LIMIT_REVOKE_SESSION_WINDOW", "5m")
	t.Setenv("RATE_LIMIT_REVOKE_OTHER_SESSIONS_MAX_REQUESTS", "3")
	t.Setenv("RATE_LIMIT_REVOKE_OTHER_SESSIONS_WINDOW", "7m")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}

	if cfg.Addr != ":9090" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.Process != ProcessWorker {
		t.Fatalf("Process = %q, want %q", cfg.Process, ProcessWorker)
	}
	if cfg.Env != "test" {
		t.Fatalf("Env = %q, want %q", cfg.Env, "test")
	}
	if cfg.LogFormat != "json" {
		t.Fatalf("LogFormat = %q, want %q", cfg.LogFormat, "json")
	}
	if cfg.DatabasePath != "/tmp/app-test.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/tmp/app-test.db")
	}
	if !cfg.CookieSecure {
		t.Fatal("CookieSecure = false, want true")
	}
	if cfg.SecretKeyBase != "csrf-signing-key" {
		t.Fatalf("SecretKeyBase = %q, want %q", cfg.SecretKeyBase, "csrf-signing-key")
	}
	if cfg.EmailVerificationRequired {
		t.Fatal("EmailVerificationRequired = true, want false")
	}
	if cfg.EmailChangeNoticeEnabled {
		t.Fatal("EmailChangeNoticeEnabled = true, want false")
	}
	if cfg.PasswordMinLength != 16 {
		t.Fatalf("PasswordMinLength = %d, want %d", cfg.PasswordMinLength, 16)
	}
	if cfg.PasswordPepper != "super-secret-pepper" {
		t.Fatalf("PasswordPepper = %q, want %q", cfg.PasswordPepper, "super-secret-pepper")
	}
	if cfg.AppBaseURL != "https://app.example.com" {
		t.Fatalf("AppBaseURL = %q, want %q", cfg.AppBaseURL, "https://app.example.com")
	}
	if cfg.EmailFrom != `"Example" <mail@example.com>` {
		t.Fatalf("EmailFrom = %q, want formatted sender", cfg.EmailFrom)
	}
	if cfg.EmailProvider != EmailProviderLog {
		t.Fatalf("EmailProvider = %q, want %q", cfg.EmailProvider, EmailProviderLog)
	}
	if !cfg.EmailLogBody {
		t.Fatal("EmailLogBody = false, want true")
	}
	if cfg.CleanupInterval != 30*time.Minute {
		t.Fatalf("CleanupInterval = %v, want %v", cfg.CleanupInterval, 30*time.Minute)
	}
	if cfg.CleanupTokenRetention != 36*time.Hour {
		t.Fatalf("CleanupTokenRetention = %v, want %v", cfg.CleanupTokenRetention, 36*time.Hour)
	}
	if cfg.CleanupSentEmailRetention != 240*time.Hour {
		t.Fatalf("CleanupSentEmailRetention = %v, want %v", cfg.CleanupSentEmailRetention, 240*time.Hour)
	}
	if cfg.CleanupFailedEmailRetention != 360*time.Hour {
		t.Fatalf("CleanupFailedEmailRetention = %v, want %v", cfg.CleanupFailedEmailRetention, 360*time.Hour)
	}
	if cfg.RateLimitPolicies.Login.MaxRequests != 7 {
		t.Fatalf("RateLimitPolicies.Login.MaxRequests = %d, want %d", cfg.RateLimitPolicies.Login.MaxRequests, 7)
	}
	if cfg.RateLimitPolicies.Login.Window != 2*time.Minute {
		t.Fatalf("RateLimitPolicies.Login.Window = %v, want %v", cfg.RateLimitPolicies.Login.Window, 2*time.Minute)
	}
	if cfg.RateLimitPolicies.ResetPassword.MaxRequests != 8 {
		t.Fatalf("RateLimitPolicies.ResetPassword.MaxRequests = %d, want %d", cfg.RateLimitPolicies.ResetPassword.MaxRequests, 8)
	}
	if cfg.RateLimitPolicies.ResetPassword.Window != 30*time.Minute {
		t.Fatalf("RateLimitPolicies.ResetPassword.Window = %v, want %v", cfg.RateLimitPolicies.ResetPassword.Window, 30*time.Minute)
	}
	if cfg.RateLimitPolicies.ChangePassword.MaxRequests != 4 {
		t.Fatalf("RateLimitPolicies.ChangePassword.MaxRequests = %d, want %d", cfg.RateLimitPolicies.ChangePassword.MaxRequests, 4)
	}
	if cfg.RateLimitPolicies.ChangePassword.Window != 20*time.Minute {
		t.Fatalf("RateLimitPolicies.ChangePassword.Window = %v, want %v", cfg.RateLimitPolicies.ChangePassword.Window, 20*time.Minute)
	}
	if cfg.RateLimitPolicies.ChangeEmail.MaxRequests != 6 {
		t.Fatalf("RateLimitPolicies.ChangeEmail.MaxRequests = %d, want %d", cfg.RateLimitPolicies.ChangeEmail.MaxRequests, 6)
	}
	if cfg.RateLimitPolicies.ChangeEmail.Window != 25*time.Minute {
		t.Fatalf("RateLimitPolicies.ChangeEmail.Window = %v, want %v", cfg.RateLimitPolicies.ChangeEmail.Window, 25*time.Minute)
	}
	if cfg.RateLimitPolicies.RevokeSession.MaxRequests != 9 {
		t.Fatalf("RateLimitPolicies.RevokeSession.MaxRequests = %d, want %d", cfg.RateLimitPolicies.RevokeSession.MaxRequests, 9)
	}
	if cfg.RateLimitPolicies.RevokeSession.Window != 5*time.Minute {
		t.Fatalf("RateLimitPolicies.RevokeSession.Window = %v, want %v", cfg.RateLimitPolicies.RevokeSession.Window, 5*time.Minute)
	}
	if cfg.RateLimitPolicies.RevokeOtherSessions.MaxRequests != 3 {
		t.Fatalf("RateLimitPolicies.RevokeOtherSessions.MaxRequests = %d, want %d", cfg.RateLimitPolicies.RevokeOtherSessions.MaxRequests, 3)
	}
	if cfg.RateLimitPolicies.RevokeOtherSessions.Window != 7*time.Minute {
		t.Fatalf("RateLimitPolicies.RevokeOtherSessions.Window = %v, want %v", cfg.RateLimitPolicies.RevokeOtherSessions.Window, 7*time.Minute)
	}
}

func TestFromEnvParsesCookieSecureBool(t *testing.T) {
	t.Setenv("APP_COOKIE_SECURE", "1")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}

	if !cfg.CookieSecure {
		t.Fatal("CookieSecure = false, want true")
	}
}

func TestFromEnvRejectsInvalidPasswordMinLength(t *testing.T) {
	t.Setenv("AUTH_PASSWORD_MIN_LENGTH", "nope")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "AUTH_PASSWORD_MIN_LENGTH") {
		t.Fatalf("FromEnv() error = %v, want AUTH_PASSWORD_MIN_LENGTH", err)
	}
}

func TestFromEnvRejectsInvalidLogFormat(t *testing.T) {
	t.Setenv("LOG_FORMAT", "yaml")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "LOG_FORMAT") {
		t.Fatalf("FromEnv() error = %v, want LOG_FORMAT context", err)
	}
}

func TestFromEnvRejectsNonPositivePasswordMinLength(t *testing.T) {
	t.Setenv("AUTH_PASSWORD_MIN_LENGTH", "0")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "AUTH_PASSWORD_MIN_LENGTH") {
		t.Fatalf("FromEnv() error = %v, want AUTH_PASSWORD_MIN_LENGTH", err)
	}
}

func TestFromEnvRejectsInvalidCookieSecureBool(t *testing.T) {
	t.Setenv("APP_COOKIE_SECURE", "sometimes")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "APP_COOKIE_SECURE") {
		t.Fatalf("FromEnv() error = %v, want APP_COOKIE_SECURE", err)
	}
}

func TestFromEnvRejectsInvalidEmailVerificationRequiredBool(t *testing.T) {
	t.Setenv("AUTH_EMAIL_VERIFICATION_REQUIRED", "sometimes")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "AUTH_EMAIL_VERIFICATION_REQUIRED") {
		t.Fatalf("FromEnv() error = %v, want AUTH_EMAIL_VERIFICATION_REQUIRED", err)
	}
}

func TestFromEnvRejectsInvalidEmailChangeNoticeEnabledBool(t *testing.T) {
	t.Setenv("AUTH_EMAIL_CHANGE_NOTICE_ENABLED", "sometimes")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "AUTH_EMAIL_CHANGE_NOTICE_ENABLED") {
		t.Fatalf("FromEnv() error = %v, want AUTH_EMAIL_CHANGE_NOTICE_ENABLED", err)
	}
}

func TestFromEnvParsesProcessModes(t *testing.T) {
	tests := []string{ProcessAll, ProcessWeb, ProcessWorker}

	for _, process := range tests {
		t.Run(process, func(t *testing.T) {
			t.Setenv("APP_PROCESS", process)

			cfg, err := FromEnv(services.DefaultPasswordMinLength)
			if err != nil {
				t.Fatalf("FromEnv() error = %v", err)
			}
			if cfg.Process != process {
				t.Fatalf("Process = %q, want %q", cfg.Process, process)
			}
		})
	}
}

func TestFromEnvRejectsInvalidProcess(t *testing.T) {
	t.Setenv("APP_PROCESS", "jobs")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "APP_PROCESS") {
		t.Fatalf("FromEnv() error = %v, want APP_PROCESS", err)
	}
}

func TestFromEnvWithProcessOverridesEnvironmentProcess(t *testing.T) {
	t.Setenv("APP_PROCESS", "jobs")

	cfg, err := FromEnvWithProcess(services.DefaultPasswordMinLength, ProcessWeb)
	if err != nil {
		t.Fatalf("FromEnvWithProcess() error = %v", err)
	}
	if cfg.Process != ProcessWeb {
		t.Fatalf("Process = %q, want %q", cfg.Process, ProcessWeb)
	}
}

func TestFromEnvWithProcessRejectsInvalidOverride(t *testing.T) {
	_, err := FromEnvWithProcess(services.DefaultPasswordMinLength, "jobs")
	if err == nil {
		t.Fatal("FromEnvWithProcess() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "process mode") {
		t.Fatalf("FromEnvWithProcess() error = %v, want process mode context", err)
	}
}

func TestFromEnvRejectsInvalidAppBaseURL(t *testing.T) {
	t.Setenv("APP_BASE_URL", "localhost:8080")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "APP_BASE_URL") {
		t.Fatalf("FromEnv() error = %v, want APP_BASE_URL", err)
	}
}

func TestFromEnvRejectsInvalidEmailFrom(t *testing.T) {
	t.Setenv("EMAIL_FROM", "not an address")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "EMAIL_FROM") {
		t.Fatalf("FromEnv() error = %v, want EMAIL_FROM", err)
	}
}

func TestFromEnvRejectsUnknownEmailProvider(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "ses")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "EMAIL_PROVIDER") {
		t.Fatalf("FromEnv() error = %v, want EMAIL_PROVIDER", err)
	}
}

func TestFromEnvRejectsInvalidRateLimitWindow(t *testing.T) {
	t.Setenv("RATE_LIMIT_FORGOT_PASSWORD_WINDOW", "tomorrow")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "RATE_LIMIT_FORGOT_PASSWORD_WINDOW") {
		t.Fatalf("FromEnv() error = %v, want RATE_LIMIT_FORGOT_PASSWORD_WINDOW", err)
	}
}

func TestFromEnvParsesSMTPProvider(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("EMAIL_FROM", "App Sender <sender@example.com>")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "mailer")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "Ignored Sender <ignored@example.com>")
	t.Setenv("SMTP_TLS", "true")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}

	if cfg.EmailProvider != EmailProviderSMTP {
		t.Fatalf("EmailProvider = %q, want %q", cfg.EmailProvider, EmailProviderSMTP)
	}
	if cfg.SMTPHost != "smtp.example.com" {
		t.Fatalf("SMTPHost = %q, want smtp.example.com", cfg.SMTPHost)
	}
	if cfg.SMTPPort != 587 {
		t.Fatalf("SMTPPort = %d, want 587", cfg.SMTPPort)
	}
	if cfg.SMTPUsername != "mailer" {
		t.Fatalf("SMTPUsername = %q, want mailer", cfg.SMTPUsername)
	}
	if cfg.SMTPPassword != "secret" {
		t.Fatalf("SMTPPassword = %q, want secret", cfg.SMTPPassword)
	}
	if cfg.EmailFrom != `"App Sender" <sender@example.com>` {
		t.Fatalf("EmailFrom = %q, want formatted sender from EMAIL_FROM", cfg.EmailFrom)
	}
	if !cfg.SMTPTLS {
		t.Fatal("SMTPTLS = false, want true")
	}
}

func TestFromEnvSMTPProviderDefaultsTLS(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("EMAIL_FROM", "Go Spark <hello@example.com>")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_TLS", "")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}

	if cfg.EmailFrom != `"Go Spark" <hello@example.com>` {
		t.Fatalf("EmailFrom = %q, want formatted sender from EMAIL_FROM", cfg.EmailFrom)
	}
	if !cfg.SMTPTLS {
		t.Fatal("SMTPTLS = false, want true default")
	}
}

func TestFromEnvSMTPProviderRejectsMissingHost(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "587")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "SMTP_HOST") {
		t.Fatalf("FromEnv() error = %v, want SMTP_HOST", err)
	}
}

func TestFromEnvSMTPProviderRejectsMissingPort(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "SMTP_PORT") {
		t.Fatalf("FromEnv() error = %v, want SMTP_PORT", err)
	}
}

func TestFromEnvSMTPProviderRejectsInvalidPort(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "abc")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "SMTP_PORT") {
		t.Fatalf("FromEnv() error = %v, want SMTP_PORT", err)
	}
}

func TestFromEnvSMTPProviderRejectsInvalidTLSBool(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_TLS", "sometimes")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "SMTP_TLS") {
		t.Fatalf("FromEnv() error = %v, want SMTP_TLS", err)
	}
}

func TestFromEnvSMTPProviderRejectsPartialAuth(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "mailer")
	t.Setenv("SMTP_PASSWORD", "")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "SMTP_USERNAME") {
		t.Fatalf("FromEnv() error = %v, want SMTP_USERNAME", err)
	}
}

func TestFromEnvLogProviderIgnoresSMTPSettings(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "log")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "not-a-number")
	t.Setenv("SMTP_TLS", "sometimes")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.EmailProvider != EmailProviderLog {
		t.Fatalf("EmailProvider = %q, want log", cfg.EmailProvider)
	}
}

func TestFromEnvParsesTrustedProxies(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_IPS", "127.0.0.1, 10.0.0.1, 192.168.0.0/24")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if len(cfg.TrustedProxies) != 3 {
		t.Fatalf("TrustedProxies length = %d, want 3", len(cfg.TrustedProxies))
	}
	if cfg.TrustedProxies[0] != "127.0.0.1" {
		t.Fatalf("TrustedProxies[0] = %q, want %q", cfg.TrustedProxies[0], "127.0.0.1")
	}
	if cfg.TrustedProxies[2] != "192.168.0.0/24" {
		t.Fatalf("TrustedProxies[2] = %q, want %q", cfg.TrustedProxies[2], "192.168.0.0/24")
	}
}

func TestFromEnvTrustedProxiesDefaultsToNil(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_IPS", "")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("TrustedProxies = %v, want empty", cfg.TrustedProxies)
	}
}

func TestFromEnvRejectsInvalidTrustedProxyIP(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_IPS", "not-an-ip")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "TRUSTED_PROXY_IPS") {
		t.Fatalf("FromEnv() error = %v, want TRUSTED_PROXY_IPS context", err)
	}
}

func TestFromEnvRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_IPS", "999.999.999.0/24")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "TRUSTED_PROXY_IPS") {
		t.Fatalf("FromEnv() error = %v, want TRUSTED_PROXY_IPS context", err)
	}
}

func TestFromEnvRejectsInvalidEmailLogBodyBool(t *testing.T) {
	t.Setenv("EMAIL_LOG_BODY", "sometimes")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "EMAIL_LOG_BODY") {
		t.Fatalf("FromEnv() error = %v, want EMAIL_LOG_BODY", err)
	}
}
