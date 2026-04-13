package config

import "os"

type Config struct {
	Addr         string
	Env          string
	DatabasePath string
}

func FromEnv() Config {
	return Config{
		Addr:         envOrDefault("APP_ADDR", ":8080"),
		Env:          envOrDefault("APP_ENV", "development"),
		DatabasePath: envOrDefault("DATABASE_PATH", "./data/app.db"),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
