package config

import (
	"os"
	"strconv"
)

const DefaultPasswordMinLength = 12

type Config struct {
	Addr              string
	Env               string
	DatabasePath      string
	CookieSecure      bool
	PasswordMinLength int
}

func FromEnv() Config {
	return Config{
		Addr:              envOrDefault("APP_ADDR", ":8080"),
		Env:               envOrDefault("APP_ENV", "development"),
		DatabasePath:      envOrDefault("DATABASE_PATH", "./data/app.db"),
		CookieSecure:      envBool("APP_COOKIE_SECURE"),
		PasswordMinLength: envIntOrDefault("AUTH_PASSWORD_MIN_LENGTH", DefaultPasswordMinLength),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && value
}

func envIntOrDefault(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
