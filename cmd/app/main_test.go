package main

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
		SMTPFrom:      "Mailer <mailer@example.com>",
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

func TestAuthSenderFromUsesSMTPFromForSMTPProvider(t *testing.T) {
	from := authSenderFrom(config.Config{
		EmailProvider: email.ProviderSMTP,
		EmailFrom:     "App <app@example.com>",
		SMTPFrom:      "Mailer <mailer@example.com>",
	})
	if from != "Mailer <mailer@example.com>" {
		t.Fatalf("authSenderFrom() = %q, want SMTP_FROM", from)
	}
}

func TestAuthSenderFromUsesEmailFromByDefault(t *testing.T) {
	from := authSenderFrom(config.Config{
		EmailProvider: email.ProviderLog,
		EmailFrom:     "App <app@example.com>",
		SMTPFrom:      "Mailer <mailer@example.com>",
	})
	if from != "App <app@example.com>" {
		t.Fatalf("authSenderFrom() = %q, want EMAIL_FROM", from)
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

func TestValidateSecurityConfigRequiresCSRFSigningKeyInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.CSRFSigningKey = ""

	assertValidateSecurityConfigErrorContains(t, cfg, "CSRF_SIGNING_KEY")
}

func TestValidateSecurityConfigRejectsWhitespaceCSRFSigningKeyInProduction(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.CSRFSigningKey = " \n\t "

	assertValidateSecurityConfigErrorContains(t, cfg, "CSRF_SIGNING_KEY")
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

func TestResolveCSRFSigningKeyUsesConfiguredValue(t *testing.T) {
	key, err := resolveCSRFSigningKey(config.Config{
		CSRFSigningKey: "configured-key",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("resolveCSRFSigningKey() error = %v", err)
	}
	if key != "configured-key" {
		t.Fatalf("resolveCSRFSigningKey() = %q, want configured key", key)
	}
}

func TestResolveCSRFSigningKeyGeneratesEphemeralWhenUnset(t *testing.T) {
	key, err := resolveCSRFSigningKey(config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("resolveCSRFSigningKey() error = %v", err)
	}
	if key == "" {
		t.Fatal("resolveCSRFSigningKey() = empty, want generated key")
	}
}

func TestProcessArgReturnsEmptyWhenNoArg(t *testing.T) {
	process, err := processArg(nil)
	if err != nil {
		t.Fatalf("processArg() error = %v", err)
	}
	if process != "" {
		t.Fatalf("processArg() = %q, want empty", process)
	}
}

func TestProcessArgReturnsValidMode(t *testing.T) {
	process, err := processArg([]string{"web"})
	if err != nil {
		t.Fatalf("processArg() error = %v", err)
	}
	if process != config.ProcessWeb {
		t.Fatalf("processArg() = %q, want %q", process, config.ProcessWeb)
	}
}

func TestProcessArgRejectsInvalidMode(t *testing.T) {
	_, err := processArg([]string{"jobs"})
	if err == nil {
		t.Fatal("processArg() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "process mode") {
		t.Fatalf("processArg() error = %v, want process mode context", err)
	}
}

func TestProcessArgRejectsMultipleArgs(t *testing.T) {
	_, err := processArg([]string{"web", "worker"})
	if err == nil {
		t.Fatal("processArg() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("processArg() error = %v, want argument count context", err)
	}
}

func productionSecurityConfig() config.Config {
	return config.Config{
		Env:            "production",
		CookieSecure:   true,
		CSRFSigningKey: "csrf-key",
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
