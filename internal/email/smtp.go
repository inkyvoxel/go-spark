package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

const ProviderSMTP = "smtp"

type SMTPSenderOptions struct {
	Logger   *slog.Logger
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
}

type SMTPSender struct {
	logger      *slog.Logger
	host        string
	port        int
	username    string
	password    string
	from        string
	useTLS      bool
	dialContext func(ctx context.Context, network, address string) (net.Conn, error)
	newClient   func(conn net.Conn, host string) (smtpClient, error)
}

type smtpClient interface {
	Extension(ext string) (bool, string)
	StartTLS(config *tls.Config) error
	Auth(auth smtp.Auth) error
	Mail(from string) error
	Rcpt(to string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

type smtpClientAdapter struct {
	client *smtp.Client
}

func (c *smtpClientAdapter) Extension(ext string) (bool, string) { return c.client.Extension(ext) }
func (c *smtpClientAdapter) StartTLS(config *tls.Config) error   { return c.client.StartTLS(config) }
func (c *smtpClientAdapter) Auth(auth smtp.Auth) error           { return c.client.Auth(auth) }
func (c *smtpClientAdapter) Mail(from string) error              { return c.client.Mail(from) }
func (c *smtpClientAdapter) Rcpt(to string) error                { return c.client.Rcpt(to) }
func (c *smtpClientAdapter) Data() (io.WriteCloser, error)       { return c.client.Data() }
func (c *smtpClientAdapter) Quit() error                         { return c.client.Quit() }
func (c *smtpClientAdapter) Close() error                        { return c.client.Close() }

func NewSMTPSender(opts SMTPSenderOptions) (*SMTPSender, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	host := strings.TrimSpace(opts.Host)
	if host == "" {
		return nil, fmt.Errorf("smtp host is required")
	}
	if opts.Port <= 0 {
		return nil, fmt.Errorf("smtp port must be greater than zero")
	}

	username := strings.TrimSpace(opts.Username)
	password := opts.Password
	if (username == "") != (password == "") {
		return nil, fmt.Errorf("smtp username and password must both be set when using authentication")
	}

	from, err := normalizeAddress("smtp from", opts.From)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &SMTPSender{
		logger:      logger,
		host:        host,
		port:        opts.Port,
		username:    username,
		password:    password,
		from:        from,
		useTLS:      opts.UseTLS,
		dialContext: dialer.DialContext,
		newClient: func(conn net.Conn, serverHost string) (smtpClient, error) {
			client, err := smtp.NewClient(conn, serverHost)
			if err != nil {
				return nil, err
			}
			return &smtpClientAdapter{client: client}, nil
		},
	}, nil
}

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fromHeader, _, err := parseAddress("from", message.From)
	if err != nil {
		return err
	}
	toHeader, toEnvelope, err := parseAddress("to", message.To)
	if err != nil {
		return err
	}

	// Keep the configured sender as the SMTP envelope sender.
	configuredEnvelope, err := envelopeAddress(s.from)
	if err != nil {
		return err
	}

	mimeMessage := Message{
		From:     fromHeader,
		To:       toHeader,
		Subject:  message.Subject,
		TextBody: message.TextBody,
		HTMLBody: message.HTMLBody,
	}
	payload, err := buildSMTPPayload(mimeMessage)
	if err != nil {
		return fmt.Errorf("build smtp payload: %w", err)
	}

	address := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	conn, err := s.dialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer conn.Close()

	client, err := s.newClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if s.useTLS {
		hasStartTLS, _ := client.Extension("STARTTLS")
		if !hasStartTLS {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName: s.host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("start tls: %w", err)
		}
	}

	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("smtp authentication failed")
		}
	}

	if err := client.Mail(configuredEnvelope); err != nil {
		return fmt.Errorf("set smtp envelope from: %w", err)
	}
	if err := client.Rcpt(toEnvelope); err != nil {
		return fmt.Errorf("set smtp recipient: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open smtp data stream: %w", err)
	}
	if _, err := writer.Write(payload); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close smtp data stream: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("close smtp session: %w", err)
	}

	s.logger.Info(
		"email sent with smtp provider",
		"provider", ProviderSMTP,
		"host", s.host,
		"port", s.port,
		"from", fromHeader,
		"to", toHeader,
		"subject", message.Subject,
	)
	return nil
}

func parseAddress(field, value string) (header string, envelope string, err error) {
	normalized, err := normalizeAddress(field, value)
	if err != nil {
		return "", "", err
	}
	envelope, err = envelopeAddress(normalized)
	if err != nil {
		return "", "", fmt.Errorf("%s email address: %w", field, err)
	}
	return normalized, envelope, nil
}

func envelopeAddress(value string) (string, error) {
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		return "", err
	}
	return parsed.Address, nil
}

func buildSMTPPayload(message Message) ([]byte, error) {
	subject := sanitizeHeaderValue(message.Subject)
	if subject == "" {
		return nil, fmt.Errorf("email subject is required")
	}

	var builder strings.Builder
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("From: " + sanitizeHeaderValue(message.From) + "\r\n")
	builder.WriteString("To: " + sanitizeHeaderValue(message.To) + "\r\n")
	builder.WriteString("Subject: " + subject + "\r\n")

	if strings.TrimSpace(message.HTMLBody) != "" {
		boundary := "go-spark-boundary"
		builder.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n")
		builder.WriteString("\r\n")

		builder.WriteString("--" + boundary + "\r\n")
		builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		builder.WriteString("Content-Transfer-Encoding: 8bit\r\n")
		builder.WriteString("\r\n")
		builder.WriteString(message.TextBody + "\r\n")

		builder.WriteString("--" + boundary + "\r\n")
		builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		builder.WriteString("Content-Transfer-Encoding: 8bit\r\n")
		builder.WriteString("\r\n")
		builder.WriteString(message.HTMLBody + "\r\n")
		builder.WriteString("--" + boundary + "--\r\n")
		return []byte(builder.String()), nil
	}

	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	builder.WriteString("\r\n")
	builder.WriteString(message.TextBody + "\r\n")
	return []byte(builder.String()), nil
}

func sanitizeHeaderValue(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return strings.TrimSpace(value)
}
