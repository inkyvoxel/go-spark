package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Addr              string
	Env               string
	DatabasePath      string
	CookieSecure      bool
	PasswordMinLength int
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

	return Config{
		Addr:              envOrDefault("APP_ADDR", ":8080"),
		Env:               envOrDefault("APP_ENV", "development"),
		DatabasePath:      envOrDefault("DATABASE_PATH", "./data/app.db"),
		CookieSecure:      cookieSecure,
		PasswordMinLength: passwordMinLength,
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
