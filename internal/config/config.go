package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	ProcessAll        = "all"
	ProcessWeb        = "web"
	ProcessWorker     = "worker"
	EmailProviderLog  = "log"
	EmailProviderSMTP = "smtp"
)

type RateLimitPolicyConfig struct {
	MaxRequests int
	Window      time.Duration
}

type RateLimitPoliciesConfig struct {
	Login                     RateLimitPolicyConfig
	Register                  RateLimitPolicyConfig
	ForgotPassword            RateLimitPolicyConfig
	ResetPassword             RateLimitPolicyConfig
	PublicResendVerification  RateLimitPolicyConfig
	AccountResendVerification RateLimitPolicyConfig
	ChangePassword            RateLimitPolicyConfig
	ChangeEmail               RateLimitPolicyConfig
	RevokeSession             RateLimitPolicyConfig
	RevokeOtherSessions       RateLimitPolicyConfig
}

type Config struct {
	Addr                        string
	Process                     string
	Env                         string
	LogFormat                   string
	DatabasePath                string
	CookieSecure                bool
	CSRFSigningKey              string
	EmailVerificationRequired   bool
	EmailChangeNoticeEnabled    bool
	PasswordMinLength           int
	PasswordPepper              string
	AppBaseURL                  string
	EmailFrom                   string
	EmailProvider               string
	EmailLogBody                bool
	SMTPHost                    string
	SMTPPort                    int
	SMTPUsername                string
	SMTPPassword                string
	SMTPTLS                     bool
	CleanupInterval             time.Duration
	CleanupTokenRetention       time.Duration
	CleanupSentEmailRetention   time.Duration
	CleanupFailedEmailRetention time.Duration
	RateLimitPolicies           RateLimitPoliciesConfig
	TrustedProxies              []string
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
	return FromEnvWithProcess(defaultPasswordMinLength, "")
}

func FromEnvWithProcess(defaultPasswordMinLength int, processOverride string) (Config, error) {
	process, err := processFromOverrideOrEnv(processOverride)
	if err != nil {
		return Config{}, err
	}
	logFormat, err := envLogFormat("LOG_FORMAT", "text")
	if err != nil {
		return Config{}, err
	}

	cookieSecure, err := envBool("APP_COOKIE_SECURE")
	if err != nil {
		return Config{}, err
	}

	passwordMinLength, err := envIntOrDefault("AUTH_PASSWORD_MIN_LENGTH", defaultPasswordMinLength)
	if err != nil {
		return Config{}, err
	}

	emailVerificationRequired, err := envBoolOrDefault("AUTH_EMAIL_VERIFICATION_REQUIRED", true)
	if err != nil {
		return Config{}, err
	}
	emailChangeNoticeEnabled, err := envBoolOrDefault("AUTH_EMAIL_CHANGE_NOTICE_ENABLED", true)
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

	emailProvider, err := envEmailProvider("EMAIL_PROVIDER", EmailProviderLog)
	if err != nil {
		return Config{}, err
	}

	emailLogBody, err := envBool("EMAIL_LOG_BODY")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:                        envOrDefault("APP_ADDR", ":8080"),
		Process:                     process,
		Env:                         envOrDefault("APP_ENV", "development"),
		LogFormat:                   logFormat,
		DatabasePath:                envOrDefault("DATABASE_PATH", "./data/app.db"),
		CookieSecure:                cookieSecure,
		CSRFSigningKey:              strings.TrimSpace(os.Getenv("CSRF_SIGNING_KEY")),
		EmailVerificationRequired:   emailVerificationRequired,
		EmailChangeNoticeEnabled:    emailChangeNoticeEnabled,
		PasswordMinLength:           passwordMinLength,
		PasswordPepper:              os.Getenv("AUTH_PASSWORD_PEPPER"),
		AppBaseURL:                  appBaseURL,
		EmailFrom:                   emailFrom,
		EmailProvider:               emailProvider,
		EmailLogBody:                emailLogBody,
		CleanupInterval:             envDurationOrDefault("JOBS_CLEANUP_INTERVAL", time.Hour),
		CleanupTokenRetention:       envDurationOrDefault("JOBS_CLEANUP_TOKEN_RETENTION", 24*time.Hour),
		CleanupSentEmailRetention:   envDurationOrDefault("JOBS_CLEANUP_SENT_EMAIL_RETENTION", 7*24*time.Hour),
		CleanupFailedEmailRetention: envDurationOrDefault("JOBS_CLEANUP_FAILED_EMAIL_RETENTION", 14*24*time.Hour),
	}

	rateLimitPolicies, err := rateLimitPoliciesFromEnv()
	if err != nil {
		return Config{}, err
	}
	cfg.RateLimitPolicies = rateLimitPolicies

	trustedProxies, err := envTrustedProxies("TRUSTED_PROXY_IPS")
	if err != nil {
		return Config{}, err
	}
	cfg.TrustedProxies = trustedProxies

	if emailProvider != EmailProviderSMTP {
		return cfg, nil
	}

	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if smtpHost == "" {
		return Config{}, fmt.Errorf("SMTP_HOST is required when EMAIL_PROVIDER=%q", EmailProviderSMTP)
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

	smtpUsername := strings.TrimSpace(os.Getenv("SMTP_USERNAME"))
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	if (smtpUsername == "") != (smtpPassword == "") {
		return Config{}, fmt.Errorf("SMTP_USERNAME and SMTP_PASSWORD must both be set when using SMTP authentication")
	}

	cfg.SMTPHost = smtpHost
	cfg.SMTPPort = smtpPort
	cfg.SMTPUsername = smtpUsername
	cfg.SMTPPassword = smtpPassword
	cfg.SMTPTLS = smtpTLS
	return cfg, nil
}

func processFromOverrideOrEnv(processOverride string) (string, error) {
	processOverride = strings.ToLower(strings.TrimSpace(processOverride))
	if processOverride != "" {
		if !IsProcess(processOverride) {
			return "", fmt.Errorf("process mode must be %q, %q, or %q", ProcessAll, ProcessWeb, ProcessWorker)
		}
		return processOverride, nil
	}

	return envProcess("APP_PROCESS", ProcessAll)
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

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
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
	if provider != EmailProviderLog && provider != EmailProviderSMTP {
		return "", fmt.Errorf("%s must be %q or %q", key, EmailProviderLog, EmailProviderSMTP)
	}

	return provider, nil
}

func envProcess(key, fallback string) (string, error) {
	process := strings.ToLower(strings.TrimSpace(envOrDefault(key, fallback)))
	if !IsProcess(process) {
		return "", fmt.Errorf("%s must be %q, %q, or %q", key, ProcessAll, ProcessWeb, ProcessWorker)
	}

	return process, nil
}

func envLogFormat(key, fallback string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(envOrDefault(key, fallback)))
	switch format {
	case "text", "json":
		return format, nil
	default:
		return "", fmt.Errorf("%s must be %q or %q", key, "text", "json")
	}
}

func IsProcess(process string) bool {
	switch process {
	case ProcessAll, ProcessWeb, ProcessWorker:
		return true
	default:
		return false
	}
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
	resetPassword, err := rateLimitPolicyFromEnv("RATE_LIMIT_RESET_PASSWORD")
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
	changePassword, err := rateLimitPolicyFromEnv("RATE_LIMIT_CHANGE_PASSWORD")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}
	changeEmail, err := rateLimitPolicyFromEnv("RATE_LIMIT_CHANGE_EMAIL")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}
	revokeSession, err := rateLimitPolicyFromEnv("RATE_LIMIT_REVOKE_SESSION")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}
	revokeOtherSessions, err := rateLimitPolicyFromEnv("RATE_LIMIT_REVOKE_OTHER_SESSIONS")
	if err != nil {
		return RateLimitPoliciesConfig{}, err
	}

	return RateLimitPoliciesConfig{
		Login:                     login,
		Register:                  register,
		ForgotPassword:            forgotPassword,
		ResetPassword:             resetPassword,
		PublicResendVerification:  publicResendVerification,
		AccountResendVerification: accountResendVerification,
		ChangePassword:            changePassword,
		ChangeEmail:               changeEmail,
		RevokeSession:             revokeSession,
		RevokeOtherSessions:       revokeOtherSessions,
	}, nil
}

func envTrustedProxies(key string) ([]string, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return nil, nil
	}
	var result []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return nil, fmt.Errorf("%s contains invalid CIDR %q: %w", key, entry, err)
			}
		} else {
			if net.ParseIP(entry) == nil {
				return nil, fmt.Errorf("%s contains invalid IP address %q", key, entry)
			}
		}
		result = append(result, entry)
	}
	return result, nil
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
