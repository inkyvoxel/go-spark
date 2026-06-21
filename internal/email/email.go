package email

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/mail"
	"net/url"
	"path"
	"strings"
	texttemplate "text/template"

	"github.com/inkyvoxel/go-spark/internal/paths"
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
	logger  *slog.Logger
	logBody bool
}

type LogSenderOptions struct {
	LogBody bool
}

func NewLogSender(logger *slog.Logger, opts LogSenderOptions) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}

	return &LogSender{
		logger:  logger,
		logBody: opts.LogBody,
	}
}

func (s *LogSender) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	attrs := []any{
		"provider", ProviderLog,
		"from", message.From,
		"to", message.To,
		"subject", message.Subject,
	}
	if s.logBody {
		attrs = append(attrs, "text_body", message.TextBody, "html_body", message.HTMLBody)
	}

	s.logger.Info("email sent with log provider", attrs...)
	return nil
}

// MessageOptions carries the process-level values every transactional email
// needs: the app's base URL (for building token links) and the From address.
// Both come from config and never vary per message. The notice email ignores
// AppBaseURL since it contains no link.
type MessageOptions struct {
	AppBaseURL string
	From       string
}

//go:embed templates/*
var emailTemplateFS embed.FS

type compiledEmailTemplates struct {
	subject *texttemplate.Template
	text    *texttemplate.Template
	html    *template.Template
}

func NewAccountConfirmationMessage(opts MessageOptions, to, token string) (Message, error) {
	from, err := normalizeAddress("from", opts.From)
	if err != nil {
		return Message{}, err
	}

	recipient, err := normalizeAddress("to", to)
	if err != nil {
		return Message{}, err
	}

	confirmURL, err := tokenURL(opts.AppBaseURL, strings.TrimPrefix(paths.ConfirmEmail, "/"), token, "confirmation")
	if err != nil {
		return Message{}, err
	}

	subject, textBody, htmlBody, err := renderEmailTemplates(
		"account_confirmation",
		struct {
			ConfirmationURL string
		}{
			ConfirmationURL: confirmURL,
		},
	)
	if err != nil {
		return Message{}, err
	}

	return Message{
		From:     from,
		To:       recipient,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}, nil
}

func NewPasswordResetMessage(opts MessageOptions, to, token string) (Message, error) {
	from, err := normalizeAddress("from", opts.From)
	if err != nil {
		return Message{}, err
	}

	recipient, err := normalizeAddress("to", to)
	if err != nil {
		return Message{}, err
	}

	resetURL, err := tokenURL(opts.AppBaseURL, strings.TrimPrefix(paths.ResetPassword, "/"), token, "password reset")
	if err != nil {
		return Message{}, err
	}

	subject, textBody, htmlBody, err := renderEmailTemplates(
		"password_reset",
		struct {
			ResetURL string
		}{
			ResetURL: resetURL,
		},
	)
	if err != nil {
		return Message{}, err
	}

	return Message{
		From:     from,
		To:       recipient,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}, nil
}

func NewEmailChangeMessage(opts MessageOptions, to, token string) (Message, error) {
	from, err := normalizeAddress("from", opts.From)
	if err != nil {
		return Message{}, err
	}

	recipient, err := normalizeAddress("to", to)
	if err != nil {
		return Message{}, err
	}

	changeURL, err := tokenURL(opts.AppBaseURL, strings.TrimPrefix(paths.ConfirmEmailChange, "/"), token, "email change")
	if err != nil {
		return Message{}, err
	}

	subject, textBody, htmlBody, err := renderEmailTemplates(
		"email_change",
		struct {
			ChangeURL string
		}{
			ChangeURL: changeURL,
		},
	)
	if err != nil {
		return Message{}, err
	}

	return Message{
		From:     from,
		To:       recipient,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}, nil
}

func NewEmailChangeNoticeMessage(opts MessageOptions, to string) (Message, error) {
	from, err := normalizeAddress("from", opts.From)
	if err != nil {
		return Message{}, err
	}

	recipient, err := normalizeAddress("to", to)
	if err != nil {
		return Message{}, err
	}

	subject, textBody, htmlBody, err := renderEmailTemplates("email_change_notice", struct{}{})
	if err != nil {
		return Message{}, err
	}

	return Message{
		From:     from,
		To:       recipient,
		Subject:  subject,
		TextBody: textBody,
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

func tokenURL(appBaseURL, path, token, tokenLabel string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("%s token is required", tokenLabel)
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

	target := base.JoinPath(path)
	query := target.Query()
	query.Set("token", token)
	target.RawQuery = query.Encode()

	return target.String(), nil
}

func renderEmailTemplates(templateName string, data any) (string, string, string, error) {
	templates, ok := emailTemplates[templateName]
	if !ok {
		return "", "", "", fmt.Errorf("unknown email template %q", templateName)
	}

	subjectOut, err := renderTextTemplate(templates.subject, data)
	if err != nil {
		return "", "", "", fmt.Errorf("render %s subject template: %w", templateName, err)
	}
	textOut, err := renderTextTemplate(templates.text, data)
	if err != nil {
		return "", "", "", fmt.Errorf("render %s text template: %w", templateName, err)
	}
	htmlOut, err := renderHTMLTemplate(templates.html, data)
	if err != nil {
		return "", "", "", fmt.Errorf("render %s html template: %w", templateName, err)
	}

	return strings.TrimSpace(subjectOut), strings.TrimSpace(textOut), strings.TrimSpace(htmlOut), nil
}

func renderTextTemplate(tpl *texttemplate.Template, data any) (string, error) {
	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func renderHTMLTemplate(tpl *template.Template, data any) (string, error) {
	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// emailTemplates are parsed once at startup from the embedded files, keyed by
// template name (e.g. "password_reset"). The files are compiled in, so a parse
// failure is an asset bug we want surfaced at boot, not deferred to a send.
var emailTemplates = mustParseEmailTemplates()

func mustParseEmailTemplates() map[string]compiledEmailTemplates {
	subjects, err := fs.Glob(emailTemplateFS, "templates/*.subject.txt")
	if err != nil {
		panic(fmt.Sprintf("glob email templates: %v", err))
	}

	compiled := make(map[string]compiledEmailTemplates, len(subjects))
	for _, subjectPath := range subjects {
		name := strings.TrimSuffix(path.Base(subjectPath), ".subject.txt")
		templates, err := parseEmailTemplateSet(name)
		if err != nil {
			panic(fmt.Sprintf("parse email templates: %v", err))
		}
		compiled[name] = templates
	}
	return compiled
}

func parseEmailTemplateSet(templateName string) (compiledEmailTemplates, error) {
	subject, err := parseTextTemplate(path.Join("templates", templateName+".subject.txt"), templateName+"-subject")
	if err != nil {
		return compiledEmailTemplates{}, fmt.Errorf("parse %s subject template: %w", templateName, err)
	}
	textBody, err := parseTextTemplate(path.Join("templates", templateName+".text.txt"), templateName+"-text")
	if err != nil {
		return compiledEmailTemplates{}, fmt.Errorf("parse %s text template: %w", templateName, err)
	}
	htmlBody, err := parseHTMLTemplate(path.Join("templates", templateName+".html.tmpl"), templateName+"-html")
	if err != nil {
		return compiledEmailTemplates{}, fmt.Errorf("parse %s html template: %w", templateName, err)
	}

	return compiledEmailTemplates{
		subject: subject,
		text:    textBody,
		html:    htmlBody,
	}, nil
}

func parseTextTemplate(filePath, name string) (*texttemplate.Template, error) {
	content, err := fs.ReadFile(emailTemplateFS, filePath)
	if err != nil {
		return nil, err
	}

	return texttemplate.New(name).Option("missingkey=error").Parse(string(content))
}

func parseHTMLTemplate(filePath, name string) (*template.Template, error) {
	content, err := fs.ReadFile(emailTemplateFS, filePath)
	if err != nil {
		return nil, err
	}

	return template.New(name).Option("missingkey=error").Parse(string(content))
}
