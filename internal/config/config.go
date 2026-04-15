package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/inkyvoxel/go-spark/internal/email"
)

type Config struct {
	Addr              string
	Env               string
	DatabasePath      string
	CookieSecure      bool
	PasswordMinLength int
	AppBaseURL        string
	EmailFrom         string
	EmailProvider     string
}

func FromEnv(defaultPasswordMinLength int) (Config, error) {
	cookieSecure, err := envBool("APP_COOKIE_SECURE")
	if err != nil {
		return Config{}, err
	}

	passwordMinLength, err := envIntOrDefault("AUTH_PASSWORD_MIN_LENGTH", defaultPasswordMinLength)
	if err != nil {
		return Config{}, err
	}

	appBaseURL, err := envURL("APP_BASE_URL", "http://localhost:8080")
	if err != nil {
		return Config{}, err
	}

	emailFrom, err := envEmailAddress("EMAIL_FROM", "Go Spark <hello@example.com>")
	if err != nil {
		return Config{}, err
	}

	emailProvider, err := envEmailProvider("EMAIL_PROVIDER", email.ProviderLog)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Addr:              envOrDefault("APP_ADDR", ":8080"),
		Env:               envOrDefault("APP_ENV", "development"),
		DatabasePath:      envOrDefault("DATABASE_PATH", "./data/app.db"),
		CookieSecure:      cookieSecure,
		PasswordMinLength: passwordMinLength,
		AppBaseURL:        appBaseURL,
		EmailFrom:         emailFrom,
		EmailProvider:     emailProvider,
	}, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return false, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}

	return parsed, nil
}

func envIntOrDefault(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}

	return value, nil
}

func envURL(key, fallback string) (string, error) {
	raw := envOrDefault(key, fallback)
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s must be a URL: %w", key, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s must use http or https", key)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%s must include a host", key)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s must not include query or fragment", key)
	}

	return strings.TrimRight(parsed.String(), "/"), nil
}

func envEmailAddress(key, fallback string) (string, error) {
	raw := envOrDefault(key, fallback)
	address, err := mail.ParseAddress(raw)
	if err != nil {
		return "", fmt.Errorf("%s must be an email address: %w", key, err)
	}

	return address.String(), nil
}

func envEmailProvider(key, fallback string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(envOrDefault(key, fallback)))
	if provider != email.ProviderLog {
		return "", fmt.Errorf("%s must be %q", key, email.ProviderLog)
	}

	return provider, nil
}
