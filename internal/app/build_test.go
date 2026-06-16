package app

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/config"
	"github.com/inkyvoxel/go-spark/internal/email"
)

func TestNewEmailSenderReturnsLogSender(t *testing.T) {
	sender, err := newEmailSender(config.Config{
		EmailProvider: email.ProviderLog,
		EmailLogBody:  true,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("newEmailSender() error = %v", err)
	}

	if _, ok := sender.(*email.LogSender); !ok {
		t.Fatalf("sender type = %T, want *email.LogSender", sender)
	}
}

func TestNewEmailSenderReturnsSMTPSender(t *testing.T) {
	sender, err := newEmailSender(config.Config{
		EmailProvider: email.ProviderSMTP,
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		EmailFrom:     "Mailer <mailer@example.com>",
		SMTPTLS:       true,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("newEmailSender() error = %v", err)
	}

	if _, ok := sender.(*email.SMTPSender); !ok {
		t.Fatalf("sender type = %T, want *email.SMTPSender", sender)
	}
}

func TestNewEmailSenderRejectsUnknownProvider(t *testing.T) {
	_, err := newEmailSender(config.Config{
		EmailProvider: "invalid",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("newEmailSender() error = nil, want error")
	}
}

func TestValidateSecurityConfigRequiresPepper(t *testing.T) {
	cfg := config.Config{PasswordPepper: ""}

	assertValidateSecurityConfigErrorContains(t, cfg, "AUTH_PASSWORD_PEPPER")
}

func TestValidateSecurityConfigRejectsWhitespacePepper(t *testing.T) {
	cfg := config.Config{PasswordPepper: " \t "}

	assertValidateSecurityConfigErrorContains(t, cfg, "AUTH_PASSWORD_PEPPER")
}

func TestValidateSecurityConfigRequiresSecretKeyBase(t *testing.T) {
	cfg := config.Config{PasswordPepper: "pepper", SecretKeyBase: ""}

	assertValidateSecurityConfigErrorContains(t, cfg, "SECRET_KEY_BASE")
}

func TestValidateSecurityConfigRejectsWhitespaceSecretKeyBase(t *testing.T) {
	cfg := config.Config{PasswordPepper: "pepper", SecretKeyBase: " \n\t "}

	assertValidateSecurityConfigErrorContains(t, cfg, "SECRET_KEY_BASE")
}

func TestValidateSecurityConfigRequiresTOTPKey(t *testing.T) {
	cfg := config.Config{PasswordPepper: "pepper", SecretKeyBase: "csrf-key", TOTPKey: ""}

	assertValidateSecurityConfigErrorContains(t, cfg, "AUTH_TOTP_KEY")
}

func TestValidateSecurityConfigRejectsWhitespaceTOTPKey(t *testing.T) {
	cfg := config.Config{PasswordPepper: "pepper", SecretKeyBase: "csrf-key", TOTPKey: " \n\t "}

	assertValidateSecurityConfigErrorContains(t, cfg, "AUTH_TOTP_KEY")
}

func TestValidateSecurityConfigAllowsRequiredSecrets(t *testing.T) {
	err := validateSecurityConfig(requiredSecurityConfig())
	if err != nil {
		t.Fatalf("validateSecurityConfig() error = %v, want nil", err)
	}
}

func TestSecurityConfigWarningsIncludesRiskySettingWarnings(t *testing.T) {
	warnings := securityConfigWarnings(config.Config{
		CookieSecure:  false,
		AppBaseURL:    "http://app.example.com",
		EmailProvider: email.ProviderLog,
		EmailLogBody:  true,
		EmailFrom:     defaultStarterEmailFrom,
	})

	if len(warnings) != 5 {
		t.Fatalf("warning count = %d, want 5", len(warnings))
	}
	assertWarningsContain(t, warnings, "APP_COOKIE_SECURE")
	assertWarningsContain(t, warnings, "APP_BASE_URL")
	assertWarningsContain(t, warnings, "EMAIL_PROVIDER")
	assertWarningsContain(t, warnings, "EMAIL_LOG_BODY")
	assertWarningsContain(t, warnings, "EMAIL_FROM")
}

func TestSecurityConfigWarningsSkipsConfiguredOptions(t *testing.T) {
	warnings := securityConfigWarnings(config.Config{
		CookieSecure:  true,
		AppBaseURL:    "https://app.example.com",
		EmailProvider: email.ProviderSMTP,
		EmailLogBody:  false,
		EmailFrom:     `"App" <security@example.com>`,
	})

	if len(warnings) != 0 {
		t.Fatalf("warning count = %d, want 0", len(warnings))
	}
}

func requiredSecurityConfig() config.Config {
	return config.Config{
		CookieSecure:   true,
		SecretKeyBase:  "csrf-key",
		TOTPKey:        "totp-key",
		AppBaseURL:     "https://app.example.com",
		PasswordPepper: "pepper",
	}
}

func assertWarningsContain(t *testing.T, warnings []string, fragment string) {
	t.Helper()

	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return
		}
	}

	t.Fatalf("warnings %q did not contain %q", warnings, fragment)
}

func assertValidateSecurityConfigErrorContains(t *testing.T, cfg config.Config, fragment string) {
	t.Helper()

	err := validateSecurityConfig(cfg)
	if err == nil {
		t.Fatal("validateSecurityConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), fragment) {
		t.Fatalf("validateSecurityConfig() error = %v, want %s context", err, fragment)
	}
}
