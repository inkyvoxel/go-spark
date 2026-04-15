package email

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"
)

const ProviderLog = "log"

type Message struct {
	From     string
	To       string
	Subject  string
	TextBody string
	HTMLBody string
}

type Sender interface {
	Send(ctx context.Context, message Message) error
}

type LogSender struct {
	logger *slog.Logger
}

func NewLogSender(logger *slog.Logger) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}

	return &LogSender{logger: logger}
}

func (s *LogSender) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.logger.Info(
		"email queued for delivery",
		"provider", ProviderLog,
		"from", message.From,
		"to", message.To,
		"subject", message.Subject,
	)
	return nil
}

type AccountConfirmationOptions struct {
	AppBaseURL string
	From       string
}

func NewAccountConfirmationMessage(opts AccountConfirmationOptions, to, token string) (Message, error) {
	from, err := normalizeAddress("from", opts.From)
	if err != nil {
		return Message{}, err
	}

	recipient, err := normalizeAddress("to", to)
	if err != nil {
		return Message{}, err
	}

	confirmURL, err := confirmationURL(opts.AppBaseURL, token)
	if err != nil {
		return Message{}, err
	}

	htmlBody, err := renderConfirmationHTML(confirmURL)
	if err != nil {
		return Message{}, err
	}

	return Message{
		From:     from,
		To:       recipient,
		Subject:  "Confirm your email address",
		TextBody: "Confirm your email address by opening this link:\n\n" + confirmURL,
		HTMLBody: htmlBody,
	}, nil
}

func normalizeAddress(field, address string) (string, error) {
	parsed, err := mail.ParseAddress(strings.TrimSpace(address))
	if err != nil {
		return "", fmt.Errorf("%s email address: %w", field, err)
	}

	return parsed.String(), nil
}

func confirmationURL(appBaseURL, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("confirmation token is required")
	}

	base, err := url.Parse(strings.TrimSpace(appBaseURL))
	if err != nil {
		return "", fmt.Errorf("app base URL: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("app base URL must use http or https")
	}
	if base.Host == "" {
		return "", fmt.Errorf("app base URL must include a host")
	}

	confirm := base.JoinPath("confirm-email")
	query := confirm.Query()
	query.Set("token", token)
	confirm.RawQuery = query.Encode()

	return confirm.String(), nil
}

func renderConfirmationHTML(confirmURL string) (string, error) {
	const body = `<p>Confirm your email address by opening this link:</p><p><a href="{{ . }}">Confirm email</a></p>`

	var rendered bytes.Buffer
	if err := template.Must(template.New("confirmation").Parse(body)).Execute(&rendered, confirmURL); err != nil {
		return "", fmt.Errorf("render confirmation email: %w", err)
	}

	return rendered.String(), nil
}
