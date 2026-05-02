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

func TestValidateSecurityConfigRequiresPepperInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.PasswordPepper = ""

	assertValidateSecurityConfigErrorContains(t, cfg, "AUTH_PASSWORD_PEPPER")
}

func TestValidateSecurityConfigRejectsWhitespacePepperInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.PasswordPepper = " \t "

	assertValidateSecurityConfigErrorContains(t, cfg, "AUTH_PASSWORD_PEPPER")
}

func TestValidateSecurityConfigRequiresCookieSecureInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.CookieSecure = false

	assertValidateSecurityConfigErrorContains(t, cfg, "APP_COOKIE_SECURE")
}

func TestValidateSecurityConfigRequiresHTTPSAppBaseURLInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.AppBaseURL = "http://app.example.com"

	assertValidateSecurityConfigErrorContains(t, cfg, "APP_BASE_URL")
}

func TestValidateSecurityConfigRequiresSecretKeyBaseInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.SecretKeyBase = ""

	assertValidateSecurityConfigErrorContains(t, cfg, "SECRET_KEY_BASE")
}

func TestValidateSecurityConfigRejectsWhitespaceSecretKeyBaseInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.SecretKeyBase = " \n\t "

	assertValidateSecurityConfigErrorContains(t, cfg, "SECRET_KEY_BASE")
}

func TestValidateSecurityConfigAllowsProductionWithSecureBaseline(t *testing.T) {
	err := validateSecurityConfig(productionSecurityConfig())
	if err != nil {
		t.Fatalf("validateSecurityConfig() error = %v, want nil", err)
	}
}

func TestValidateSecurityConfigAllowsNonProductionWithoutPepper(t *testing.T) {
	err := validateSecurityConfig(config.Config{
		Env:            "development",
		PasswordPepper: "",
	})
	if err != nil {
		t.Fatalf("validateSecurityConfig() error = %v, want nil", err)
	}
}

func TestSecurityConfigWarningsProductionIncludesOptionalSettingWarnings(t *testing.T) {
	warnings := securityConfigWarnings(config.Config{
		Env:                       "production",
		EmailVerificationRequired: false,
		EmailProvider:             email.ProviderLog,
		EmailLogBody:              true,
		EmailFrom:                 defaultStarterEmailFrom,
	})

	if len(warnings) != 4 {
		t.Fatalf("warning count = %d, want 4", len(warnings))
	}
	assertWarningsContain(t, warnings, "AUTH_EMAIL_VERIFICATION_REQUIRED")
	assertWarningsContain(t, warnings, "EMAIL_PROVIDER")
	assertWarningsContain(t, warnings, "EMAIL_LOG_BODY")
	assertWarningsContain(t, warnings, "EMAIL_FROM")
}

func TestSecurityConfigWarningsProductionSkipsConfiguredOptions(t *testing.T) {
	warnings := securityConfigWarnings(config.Config{
		Env:                       "production",
		EmailVerificationRequired: true,
		EmailProvider:             email.ProviderSMTP,
		EmailLogBody:              false,
		EmailFrom:                 `"App" <security@example.com>`,
	})

	if len(warnings) != 0 {
		t.Fatalf("warning count = %d, want 0", len(warnings))
	}
}

func TestSecurityConfigWarningsNonProductionReturnsNone(t *testing.T) {
	warnings := securityConfigWarnings(config.Config{
		Env:                       "development",
		EmailVerificationRequired: false,
		EmailProvider:             email.ProviderLog,
		EmailLogBody:              true,
		EmailFrom:                 defaultStarterEmailFrom,
	})

	if len(warnings) != 0 {
		t.Fatalf("warning count = %d, want 0", len(warnings))
	}
}

func TestResolveSecretKeyBaseUsesConfiguredValue(t *testing.T) {
	key, err := resolveSecretKeyBase(config.Config{
		SecretKeyBase: "configured-key",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("resolveSecretKeyBase() error = %v", err)
	}
	if key != "configured-key" {
		t.Fatalf("resolveSecretKeyBase() = %q, want configured key", key)
	}
}

func TestResolveSecretKeyBaseRequiresConfiguredValue(t *testing.T) {
	_, err := resolveSecretKeyBase(config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("resolveSecretKeyBase() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "SECRET_KEY_BASE") {
		t.Fatalf("resolveSecretKeyBase() error = %v, want SECRET_KEY_BASE context", err)
	}
}

func productionSecurityConfig() config.Config {
	return config.Config{
		Env:            "production",
		CookieSecure:   true,
		SecretKeyBase:  "csrf-key",
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
