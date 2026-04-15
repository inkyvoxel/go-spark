package config

import (
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/email"
	"github.com/inkyvoxel/go-spark/internal/services"
)

func TestFromEnvUsesDefaults(t *testing.T) {
	t.Setenv("APP_ADDR", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("APP_COOKIE_SECURE", "")
	t.Setenv("AUTH_PASSWORD_MIN_LENGTH", "")
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("EMAIL_PROVIDER", "")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.Env != "development" {
		t.Fatalf("Env = %q, want %q", cfg.Env, "development")
	}
	if cfg.DatabasePath != "./data/app.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "./data/app.db")
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure = true, want false")
	}
	if cfg.PasswordMinLength != services.DefaultPasswordMinLength {
		t.Fatalf("PasswordMinLength = %d, want %d", cfg.PasswordMinLength, services.DefaultPasswordMinLength)
	}
	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Fatalf("AppBaseURL = %q, want %q", cfg.AppBaseURL, "http://localhost:8080")
	}
	if cfg.EmailFrom != `"Go Spark" <hello@example.com>` {
		t.Fatalf("EmailFrom = %q, want formatted sender", cfg.EmailFrom)
	}
	if cfg.EmailProvider != email.ProviderLog {
		t.Fatalf("EmailProvider = %q, want %q", cfg.EmailProvider, email.ProviderLog)
	}
}

func TestFromEnvUsesEnvironment(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_PATH", "/tmp/app-test.db")
	t.Setenv("APP_COOKIE_SECURE", "true")
	t.Setenv("AUTH_PASSWORD_MIN_LENGTH", "16")
	t.Setenv("APP_BASE_URL", "https://app.example.com/")
	t.Setenv("EMAIL_FROM", "Example <mail@example.com>")
	t.Setenv("EMAIL_PROVIDER", "LOG")

	cfg, err := FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}

	if cfg.Addr != ":9090" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.Env != "test" {
		t.Fatalf("Env = %q, want %q", cfg.Env, "test")
	}
	if cfg.DatabasePath != "/tmp/app-test.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/tmp/app-test.db")
	}
	if !cfg.CookieSecure {
		t.Fatal("CookieSecure = false, want true")
	}
	if cfg.PasswordMinLength != 16 {
		t.Fatalf("PasswordMinLength = %d, want %d", cfg.PasswordMinLength, 16)
	}
	if cfg.AppBaseURL != "https://app.example.com" {
		t.Fatalf("AppBaseURL = %q, want %q", cfg.AppBaseURL, "https://app.example.com")
	}
	if cfg.EmailFrom != `"Example" <mail@example.com>` {
		t.Fatalf("EmailFrom = %q, want formatted sender", cfg.EmailFrom)
	}
	if cfg.EmailProvider != email.ProviderLog {
		t.Fatalf("EmailProvider = %q, want %q", cfg.EmailProvider, email.ProviderLog)
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
	t.Setenv("EMAIL_PROVIDER", "smtp")

	_, err := FromEnv(services.DefaultPasswordMinLength)
	if err == nil {
		t.Fatal("FromEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "EMAIL_PROVIDER") {
		t.Fatalf("FromEnv() error = %v, want EMAIL_PROVIDER", err)
	}
}
