package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/inkyvoxel/go-spark/internal/email"
	"github.com/joho/godotenv"
)

type RateLimitPolicyConfig struct {
	MaxRequests int
	Window      time.Duration
}

type RateLimitPoliciesConfig struct {
	Login                     RateLimitPolicyConfig
	Register                  RateLimitPolicyConfig
	ForgotPassword            RateLimitPolicyConfig
	PublicResendVerification  RateLimitPolicyConfig
	AccountResendVerification RateLimitPolicyConfig
}

type Config struct {
	Addr              string
	Env               string
	DatabasePath      string
	CookieSecure      bool
	PasswordMinLength int
	PasswordPepper    string
	AppBaseURL        string
	EmailFrom         string
	EmailProvider     string
	EmailLogBody      bool
	SMTPHost          string
	SMTPPort          int
	SMTPUsername      string
	SMTPPassword      string
	SMTPFrom          string
	SMTPTLS           bool
	RateLimitPolicies RateLimitPoliciesConfig
}

func LoadDotEnv(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}

	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}

	return nil
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

	emailLogBody, err := envBool("EMAIL_LOG_BODY")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:              envOrDefault("APP_ADDR", ":8080"),
		Env:               envOrDefault("APP_ENV", "development"),
		DatabasePath:      envOrDefault("DATABASE_PATH", "./data/app.db"),
		CookieSecure:      cookieSecure,
		PasswordMinLength: passwordMinLength,
		PasswordPepper:    os.Getenv("AUTH_PASSWORD_PEPPER"),
		AppBaseURL:        appBaseURL,
		EmailFrom:         emailFrom,
		EmailProvider:     emailProvider,
		EmailLogBody:      emailLogBody,
	}

	rateLimitPolicies, err := rateLimitPoliciesFromEnv()
	if err != nil {
		return Config{}, err
	}
	cfg.RateLimitPolicies = rateLimitPolicies

	if emailProvider != email.ProviderSMTP {
		return cfg, nil
	}

	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if smtpHost == "" {
		return Config{}, fmt.Errorf("SMTP_HOST is required when EMAIL_PROVIDER=%q", email.ProviderSMTP)
	}

	smtpPort, err := envInt("SMTP_PORT")
	if err != nil {
		return Config{}, err
	}
	if smtpPort <= 0 {
		return Config{}, fmt.Errorf("SMTP_PORT must be greater than zero")
	}

	smtpTLS, err := envBoolOrDefault("SMTP_TLS", true)
	if err != nil {
		return Config{}, err
	}

	smtpFrom, err := envEmailAddress("SMTP_FROM", emailFrom)
	if err != nil {
		return Config{}, err
	}

	smtpUsername := strings.TrimSpace(os.Getenv("SMTP_USERNAME"))
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	if (smtpUsername == "") != (smtpPassword == "") {
		return Config{}, fmt.Errorf("SMTP_USERNAME and SMTP_PASSWORD must both be set when using SMTP authentication")
	}

	cfg.SMTPHost = smtpHost
	cfg.SMTPPort = smtpPort
	cfg.SMTPUsername = smtpUsername
	cfg.SMTPPassword = smtpPassword
	cfg.SMTPFrom = smtpFrom
	cfg.SMTPTLS = smtpTLS
	return cfg, nil
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

func envBoolOrDefault(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
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

func envInt(key string) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", key)
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func envIntOptionalPositive(key string) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, nil
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

func envDurationOptionalPositive(key string) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
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
	if provider != email.ProviderLog && provider != email.ProviderSMTP {
		return "", fmt.Errorf("%s must be %q or %q", key, email.ProviderLog, email.ProviderSMTP)
	}

	return provider, nil
}

func rateLimitPoliciesFromEnv() (RateLimitPoliciesConfig, error) {
	login, err := rateLimitPolicyFromEnv("RATE_LIMIT_LOGIN")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}
	register, err := rateLimitPolicyFromEnv("RATE_LIMIT_REGISTER")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}
	forgotPassword, err := rateLimitPolicyFromEnv("RATE_LIMIT_FORGOT_PASSWORD")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}
	publicResendVerification, err := rateLimitPolicyFromEnv("RATE_LIMIT_PUBLIC_RESEND_VERIFICATION")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}
	accountResendVerification, err := rateLimitPolicyFromEnv("RATE_LIMIT_ACCOUNT_RESEND_VERIFICATION")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}

	return RateLimitPoliciesConfig{
		Login:                     login,
		Register:                  register,
		ForgotPassword:            forgotPassword,
		PublicResendVerification:  publicResendVerification,
		AccountResendVerification: accountResendVerification,
	}, nil
}

func rateLimitPolicyFromEnv(prefix string) (RateLimitPolicyConfig, error) {
	maxRequests, err := envIntOptionalPositive(prefix + "_MAX_REQUESTS")
	if err != nil {
		return RateLimitPolicyConfig{}, err
	}
	window, err := envDurationOptionalPositive(prefix + "_WINDOW")
	if err != nil {
		return RateLimitPolicyConfig{}, err
	}

	return RateLimitPolicyConfig{
		MaxRequests: maxRequests,
		Window:      window,
	}, nil
}
