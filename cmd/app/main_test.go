package main

import (
	"io"
	"log/slog"
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
