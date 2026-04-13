package config

import "testing"

func TestFromEnvUsesDefaults(t *testing.T) {
	t.Setenv("APP_ADDR", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("DATABASE_PATH", "")

	cfg := FromEnv()

	if cfg.Addr != ":8080" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.Env != "development" {
		t.Fatalf("Env = %q, want %q", cfg.Env, "development")
	}
	if cfg.DatabasePath != "./data/app.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "./data/app.db")
	}
}

func TestFromEnvUsesEnvironment(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_PATH", "/tmp/app-test.db")

	cfg := FromEnv()

	if cfg.Addr != ":9090" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, ":9090")
	}
	if cfg.Env != "test" {
		t.Fatalf("Env = %q, want %q", cfg.Env, "test")
	}
	if cfg.DatabasePath != "/tmp/app-test.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/tmp/app-test.db")
	}
}
