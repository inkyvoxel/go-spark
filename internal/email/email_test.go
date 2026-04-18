package email

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewAccountConfirmationMessage(t *testing.T) {
	message, err := NewAccountConfirmationMessage(AccountConfirmationOptions{
		AppBaseURL: "https://app.example.com",
		From:       "Go Spark <hello@example.com>",
	}, "USER@example.com", "token value")
	if err != nil {
		t.Fatalf("NewAccountConfirmationMessage() error = %v", err)
	}

	if message.From != `"Go Spark" <hello@example.com>` {
		t.Fatalf("From = %q, want formatted sender", message.From)
	}
	if message.To != "<USER@example.com>" {
		t.Fatalf("To = %q, want formatted recipient", message.To)
	}
	if message.Subject != "Confirm your email address" {
		t.Fatalf("Subject = %q, want confirmation subject", message.Subject)
	}
	if !strings.Contains(message.TextBody, "https://app.example.com/account/confirm-email?token=token+value") {
		t.Fatalf("TextBody = %q, want confirmation URL", message.TextBody)
	}
	if !strings.Contains(message.HTMLBody, `href="https://app.example.com/account/confirm-email?token=token&#43;value"`) {
		t.Fatalf("HTMLBody = %q, want escaped confirmation URL", message.HTMLBody)
	}
}

func TestNewAccountConfirmationMessageKeepsBasePath(t *testing.T) {
	message, err := NewAccountConfirmationMessage(AccountConfirmationOptions{
		AppBaseURL: "https://example.com/app",
		From:       "hello@example.com",
	}, "user@example.com", "token")
	if err != nil {
		t.Fatalf("NewAccountConfirmationMessage() error = %v", err)
	}

	if !strings.Contains(message.TextBody, "https://example.com/app/account/confirm-email?token=token") {
		t.Fatalf("TextBody = %q, want confirmation URL under base path", message.TextBody)
	}
}

func TestNewAccountConfirmationMessageValidatesInputs(t *testing.T) {
	tests := []struct {
		name  string
		opts  AccountConfirmationOptions
		to    string
		token string
		want  string
	}{
		{
			name:  "invalid from",
			opts:  AccountConfirmationOptions{AppBaseURL: "https://app.example.com", From: "not an address"},
			to:    "user@example.com",
			token: "token",
			want:  "from email address",
		},
		{
			name:  "invalid to",
			opts:  AccountConfirmationOptions{AppBaseURL: "https://app.example.com", From: "hello@example.com"},
			to:    "nope",
			token: "token",
			want:  "to email address",
		},
		{
			name:  "invalid base URL",
			opts:  AccountConfirmationOptions{AppBaseURL: "localhost:8080", From: "hello@example.com"},
			to:    "user@example.com",
			token: "token",
			want:  "app base URL",
		},
		{
			name:  "empty token",
			opts:  AccountConfirmationOptions{AppBaseURL: "https://app.example.com", From: "hello@example.com"},
			to:    "user@example.com",
			token: " ",
			want:  "confirmation token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAccountConfirmationMessage(tt.opts, tt.to, tt.token)
			if err == nil {
				t.Fatal("NewAccountConfirmationMessage() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewAccountConfirmationMessage() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNewPasswordResetMessage(t *testing.T) {
	message, err := NewPasswordResetMessage(PasswordResetOptions{
		AppBaseURL: "https://app.example.com",
		From:       "Go Spark <hello@example.com>",
	}, "USER@example.com", "token value")
	if err != nil {
		t.Fatalf("NewPasswordResetMessage() error = %v", err)
	}

	if message.From != `"Go Spark" <hello@example.com>` {
		t.Fatalf("From = %q, want formatted sender", message.From)
	}
	if message.To != "<USER@example.com>" {
		t.Fatalf("To = %q, want formatted recipient", message.To)
	}
	if message.Subject != "Reset your password" {
		t.Fatalf("Subject = %q, want password reset subject", message.Subject)
	}
	if !strings.Contains(message.TextBody, "https://app.example.com/account/reset-password?token=token+value") {
		t.Fatalf("TextBody = %q, want password reset URL", message.TextBody)
	}
	if !strings.Contains(message.HTMLBody, `href="https://app.example.com/account/reset-password?token=token&#43;value"`) {
		t.Fatalf("HTMLBody = %q, want escaped password reset URL", message.HTMLBody)
	}
}

func TestNewPasswordResetMessageValidatesInputs(t *testing.T) {
	tests := []struct {
		name  string
		opts  PasswordResetOptions
		to    string
		token string
		want  string
	}{
		{
			name:  "invalid from",
			opts:  PasswordResetOptions{AppBaseURL: "https://app.example.com", From: "not an address"},
			to:    "user@example.com",
			token: "token",
			want:  "from email address",
		},
		{
			name:  "invalid to",
			opts:  PasswordResetOptions{AppBaseURL: "https://app.example.com", From: "hello@example.com"},
			to:    "nope",
			token: "token",
			want:  "to email address",
		},
		{
			name:  "invalid base URL",
			opts:  PasswordResetOptions{AppBaseURL: "localhost:8080", From: "hello@example.com"},
			to:    "user@example.com",
			token: "token",
			want:  "app base URL",
		},
		{
			name:  "empty token",
			opts:  PasswordResetOptions{AppBaseURL: "https://app.example.com", From: "hello@example.com"},
			to:    "user@example.com",
			token: " ",
			want:  "password reset token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPasswordResetMessage(tt.opts, tt.to, tt.token)
			if err == nil {
				t.Fatal("NewPasswordResetMessage() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewPasswordResetMessage() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLogSenderDoesNotLogMessageBodies(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	sender := NewLogSender(logger, LogSenderOptions{})

	err := sender.Send(context.Background(), Message{
		From:     "hello@example.com",
		To:       "user@example.com",
		Subject:  "Confirm your email address",
		TextBody: "secret token",
		HTMLBody: "<a href=\"https://example.com?token=secret\">Confirm</a>",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	logged := output.String()
	if !strings.Contains(logged, "Confirm your email address") {
		t.Fatalf("log output = %q, want subject", logged)
	}
	if strings.Contains(logged, "secret token") || strings.Contains(logged, "token=secret") {
		t.Fatalf("log output = %q, did not want message body", logged)
	}
}

func TestLogSenderLogsMessageBodiesWhenEnabled(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	sender := NewLogSender(logger, LogSenderOptions{LogBody: true})

	err := sender.Send(context.Background(), Message{
		From:     "hello@example.com",
		To:       "user@example.com",
		Subject:  "Confirm your email address",
		TextBody: "Confirm here: http://localhost:8080/account/confirm-email?token=secret",
		HTMLBody: `<a href="http://localhost:8080/account/confirm-email?token=secret">Confirm</a>`,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	logged := output.String()
	if !strings.Contains(logged, "token=secret") {
		t.Fatalf("log output = %q, want message body", logged)
	}
}

func TestLogSenderReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewLogSender(nil, LogSenderOptions{}).Send(ctx, Message{})
	if err == nil {
		t.Fatal("Send() error = nil, want context error")
	}
}
