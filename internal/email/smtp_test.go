package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

func TestNewSMTPSenderValidatesRequiredFields(t *testing.T) {
	_, err := NewSMTPSender(SMTPSenderOptions{
		Host: "smtp.example.com",
		Port: 587,
		From: "not an address",
	})
	if err == nil {
		t.Fatal("NewSMTPSender() error = nil, want error")
	}

	_, err = NewSMTPSender(SMTPSenderOptions{
		Host:     "smtp.example.com",
		Port:     587,
		From:     "Mailer <mailer@example.com>",
		Username: "mailer",
	})
	if err == nil {
		t.Fatal("NewSMTPSender() partial auth error = nil, want error")
	}
}

func TestBuildSMTPPayloadMultipartContainsBothBodies(t *testing.T) {
	payload, err := buildSMTPPayload(Message{
		From:     `"Mailer" <mailer@example.com>`,
		To:       "<user@example.com>",
		Subject:  "Confirm your email",
		TextBody: "Text content",
		HTMLBody: "<p>HTML content</p>",
	})
	if err != nil {
		t.Fatalf("buildSMTPPayload() error = %v", err)
	}

	body := string(payload)
	for _, want := range []string{
		"Content-Type: multipart/alternative;",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Type: text/html; charset=UTF-8",
		"Text content",
		"<p>HTML content</p>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("payload missing %q in %q", want, body)
		}
	}
}

func TestSMTPSenderSendRequiresStartTLSWhenEnabled(t *testing.T) {
	sender, err := NewSMTPSender(SMTPSenderOptions{
		Host:   "smtp.example.com",
		Port:   587,
		From:   "Mailer <mailer@example.com>",
		UseTLS: true,
	})
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	sender.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return &stubConn{}, nil
	}
	sender.newClient = func(conn net.Conn, host string) (smtpClient, error) {
		return &fakeSMTPClient{}, nil
	}

	err = sender.Send(context.Background(), Message{
		From:     "Mailer <mailer@example.com>",
		To:       "user@example.com",
		Subject:  "Confirm",
		TextBody: "Body",
	})
	if err == nil {
		t.Fatal("Send() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("Send() error = %v, want STARTTLS context", err)
	}
}

func TestSMTPSenderSendDoesNotLeakPasswordInErrors(t *testing.T) {
	sender, err := NewSMTPSender(SMTPSenderOptions{
		Host:     "smtp.example.com",
		Port:     587,
		From:     "Mailer <mailer@example.com>",
		UseTLS:   true,
		Username: "mailer",
		Password: "super-secret-password",
	})
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	sender.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return &stubConn{}, nil
	}
	sender.newClient = func(conn net.Conn, host string) (smtpClient, error) {
		return &fakeSMTPClient{
			startTLSSupported: true,
			authErr:           errors.New("backend auth failed: super-secret-password"),
		}, nil
	}

	err = sender.Send(context.Background(), Message{
		From:     "Mailer <mailer@example.com>",
		To:       "user@example.com",
		Subject:  "Confirm",
		TextBody: "Body",
	})
	if err == nil {
		t.Fatal("Send() error = nil, want error")
	}
	if strings.Contains(err.Error(), "super-secret-password") {
		t.Fatalf("Send() error leaked password: %v", err)
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("Send() error = %v, want auth context", err)
	}
}

func TestSMTPSenderSendLogsWithoutBodies(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	sender, err := NewSMTPSender(SMTPSenderOptions{
		Logger: logger,
		Host:   "smtp.example.com",
		Port:   587,
		From:   "Mailer <mailer@example.com>",
		UseTLS: false,
	})
	if err != nil {
		t.Fatalf("NewSMTPSender() error = %v", err)
	}

	sender.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return &stubConn{}, nil
	}
	sender.newClient = func(conn net.Conn, host string) (smtpClient, error) {
		return &fakeSMTPClient{dataWriter: &stubWriteCloser{}}, nil
	}

	err = sender.Send(context.Background(), Message{
		From:     "Mailer <mailer@example.com>",
		To:       "user@example.com",
		Subject:  "Confirm",
		TextBody: "Body with token=secret",
		HTMLBody: "<p>token=secret</p>",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	logged := output.String()
	if strings.Contains(logged, "token=secret") {
		t.Fatalf("log output leaked body: %q", logged)
	}
	if !strings.Contains(logged, "email sent with smtp provider") {
		t.Fatalf("log output = %q, want smtp send log", logged)
	}
}

type fakeSMTPClient struct {
	startTLSSupported bool
	startTLSErr       error
	authErr           error
	mailErr           error
	rcptErr           error
	dataErr           error
	quitErr           error
	dataWriter        io.WriteCloser
}

func (c *fakeSMTPClient) Extension(ext string) (bool, string) {
	if ext == "STARTTLS" && c.startTLSSupported {
		return true, ""
	}
	return false, ""
}

func (c *fakeSMTPClient) StartTLS(config *tls.Config) error {
	return c.startTLSErr
}

func (c *fakeSMTPClient) Auth(auth smtp.Auth) error {
	return c.authErr
}

func (c *fakeSMTPClient) Mail(from string) error {
	return c.mailErr
}

func (c *fakeSMTPClient) Rcpt(to string) error {
	return c.rcptErr
}

func (c *fakeSMTPClient) Data() (io.WriteCloser, error) {
	if c.dataErr != nil {
		return nil, c.dataErr
	}
	if c.dataWriter != nil {
		return c.dataWriter, nil
	}
	return &stubWriteCloser{}, nil
}

func (c *fakeSMTPClient) Quit() error {
	return c.quitErr
}

func (c *fakeSMTPClient) Close() error {
	return nil
}

type stubWriteCloser struct{}

func (w *stubWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (w *stubWriteCloser) Close() error                { return nil }

type stubConn struct{}

func (c *stubConn) Read(b []byte) (int, error)         { return 0, io.EOF }
func (c *stubConn) Write(b []byte) (int, error)        { return len(b), nil }
func (c *stubConn) Close() error                       { return nil }
func (c *stubConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *stubConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c *stubConn) SetDeadline(t time.Time) error      { return nil }
func (c *stubConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *stubConn) SetWriteDeadline(t time.Time) error { return nil }
