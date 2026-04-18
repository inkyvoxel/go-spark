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
	err := validateSecurityConfig(config.Config{
		Env:            "production",
		PasswordPepper: "",
	})
	if err == nil {
		t.Fatal("validateSecurityConfig() error = nil, want error")
	}
}

func TestValidateSecurityConfigAllowsProductionWithPepper(t *testing.T) {
	err := validateSecurityConfig(config.Config{
		Env:            "production",
		PasswordPepper: "pepper",
	})
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
