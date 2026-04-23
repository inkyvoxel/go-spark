package projectinit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUpdatesStarterDefaults(t *testing.T) {
	repoRoot := t.TempDir()
	writeFixtureFile(t, repoRoot, "go.mod", "module github.com/inkyvoxel/go-spark\n\ngo 1.24.0\n")
	writeFixtureFile(t, repoRoot, "README.md", "# Go Spark\n\nRun `./go-spark all` or `./go-spark serve`.\n")
	writeFixtureFile(t, repoRoot, ".env.example", strings.Join([]string{
		"DATABASE_PATH=./data/app.db",
		"AUTH_EMAIL_VERIFICATION_REQUIRED=true",
		"EMAIL_FROM=\"Go Spark <hello@example.com>\"",
		"",
	}, "\n"))
	writeFixtureFile(t, repoRoot, "Makefile", "DB_PATH ?= ./data/app.db\n")
	writeFixtureFile(t, repoRoot, "CONTRIBUTING.md", "Thanks for taking an interest in Go Spark.\nmake migrate-up DB_PATH=/tmp/go-spark-contrib.db\n")
	writeFixtureFile(t, repoRoot, "docs/architecture.md", "Go Spark prefers SQLite.\n")
	writeFixtureFile(t, repoRoot, "docs/jobs.md", "Go Spark uses jobs.\n")
	writeFixtureFile(t, repoRoot, "docs/todo.md", "starter todo\n")
	writeFixtureFile(t, repoRoot, "templates/layout.html", "<a>Go Spark</a>\n")
	writeFixtureFile(t, repoRoot, "templates/home.html", "Go Spark gives you a starter.\n")
	writeFixtureFile(t, repoRoot, "internal/server/server.go", "package server\n\nfunc title() string {\n\treturn \"Go Spark\"\n}\n")
	writeFixtureFile(t, repoRoot, "internal/config/config.go", "package config\n\nconst from = \"Go Spark <hello@example.com>\"\n")
	writeFixtureFile(t, repoRoot, "internal/app/build.go", "package app\n\nconst from = \"Go Spark <hello@example.com>\"\n")
	writeFixtureFile(t, repoRoot, "cmd/app/main.go", "package main\n\nimport _ \"github.com/inkyvoxel/go-spark/internal/app\"\n")

	var stdout bytes.Buffer
	emailVerificationRequired := false
	err := Run(repoRoot, Options{
		ProjectName:               "Acme Starter",
		ModulePath:                "github.com/acme/acme-starter",
		AppName:                   "Acme Portal",
		EmailFromName:             "Acme Portal",
		EmailFromAddress:          "team@acme.test",
		DatabasePath:              "./data/acme.db",
		EmailVerificationRequired: &emailVerificationRequired,
	}, strings.NewReader(""), &stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	assertFileContains(t, repoRoot, "go.mod", "module github.com/acme/acme-starter")
	assertFileContains(t, repoRoot, "README.md", "# Acme Starter")
	assertFileContains(t, repoRoot, "README.md", "./acme-starter all")
	assertFileContains(t, repoRoot, "README.md", "./acme-starter serve")
	assertFileContains(t, repoRoot, ".env.example", "DATABASE_PATH=./data/acme.db")
	assertFileContains(t, repoRoot, ".env.example", "AUTH_EMAIL_VERIFICATION_REQUIRED=false")
	assertFileContains(t, repoRoot, ".env.example", "EMAIL_FROM=\"Acme Portal <team@acme.test>\"")
	assertFileContains(t, repoRoot, "Makefile", "DB_PATH ?= ./data/acme.db")
	assertFileContains(t, repoRoot, "CONTRIBUTING.md", "/tmp/acme-starter-contrib.db")
	assertFileContains(t, repoRoot, "templates/layout.html", "Acme Portal")
	assertFileContains(t, repoRoot, "templates/home.html", "Welcome to Acme Portal.")
	assertFileContains(t, repoRoot, "internal/server/server.go", "\"Acme Portal\"")
	assertFileContains(t, repoRoot, "internal/config/config.go", "Acme Portal <team@acme.test>")
	assertFileContains(t, repoRoot, "internal/app/build.go", "Acme Portal <team@acme.test>")
	assertFileContains(t, repoRoot, "cmd/app/main.go", "github.com/acme/acme-starter/internal/app")
	assertFileContains(t, repoRoot, "docs/todo.md", "starter todo")
	assertFileContains(t, repoRoot, stateFileName, "DATABASE_PATH=./data/acme.db")
	assertFileContains(t, repoRoot, stateFileName, "AUTH_EMAIL_VERIFICATION_REQUIRED=false")
	assertFileNotContains(t, repoRoot, stateFileName, "TRIM_STARTER_CONTENT=")
}

func TestRunPromptsForMissingValues(t *testing.T) {
	repoRoot := t.TempDir()
	writeFixtureFile(t, repoRoot, "go.mod", "module github.com/inkyvoxel/go-spark\n")
	writeFixtureFile(t, repoRoot, "README.md", "# Go Spark\n")
	writeFixtureFile(t, repoRoot, ".env.example", strings.Join([]string{
		"DATABASE_PATH=./data/app.db",
		"AUTH_EMAIL_VERIFICATION_REQUIRED=true",
		"EMAIL_FROM=\"Go Spark <hello@example.com>\"",
		"",
	}, "\n"))
	writeFixtureFile(t, repoRoot, "Makefile", "DB_PATH ?= ./data/app.db\n")
	writeFixtureFile(t, repoRoot, "CONTRIBUTING.md", "Go Spark\n")
	writeFixtureFile(t, repoRoot, "docs/architecture.md", "Go Spark\n")
	writeFixtureFile(t, repoRoot, "docs/jobs.md", "Go Spark\n")
	writeFixtureFile(t, repoRoot, "docs/todo.md", "starter todo\n")
	writeFixtureFile(t, repoRoot, "templates/layout.html", "Go Spark\n")
	writeFixtureFile(t, repoRoot, "templates/home.html", "Go Spark\n")
	writeFixtureFile(t, repoRoot, "internal/server/server.go", "\"Go Spark\"\n")
	writeFixtureFile(t, repoRoot, "internal/config/config.go", "\"Go Spark <hello@example.com>\"\n")
	writeFixtureFile(t, repoRoot, "internal/app/build.go", "\"Go Spark <hello@example.com>\"\n")

	input := strings.Join([]string{
		"My App",
		"github.com/example/my-app",
		"My App",
		"My App",
		"hello@example.com",
		"./data/my-app.db",
		"yes",
		"",
	}, "\n")

	var stdout bytes.Buffer
	err := Run(repoRoot, Options{}, strings.NewReader(input), &stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	assertFileContains(t, repoRoot, "go.mod", "module github.com/example/my-app")
	assertFileContains(t, repoRoot, ".env.example", "DATABASE_PATH=./data/my-app.db")
	assertFileContains(t, repoRoot, "Makefile", "DB_PATH ?= ./data/my-app.db")
	if !strings.Contains(stdout.String(), "Project name [Go Spark]: ") {
		t.Fatalf("stdout = %q, want prompts", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Default database path [./data/app.db]: ") {
		t.Fatalf("stdout = %q, want database prompt", stdout.String())
	}
	if strings.Contains(stdout.String(), "Trim starter docs and example content") {
		t.Fatalf("stdout = %q, do not want cleanup prompt", stdout.String())
	}
}

func writeFixtureFile(t *testing.T, root, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", fullPath, err)
	}
}

func assertFileContains(t *testing.T, root, relativePath, fragment string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
	}
	if !strings.Contains(string(content), fragment) {
		t.Fatalf("%s = %q, want fragment %q", relativePath, string(content), fragment)
	}
}

func assertFileNotContains(t *testing.T, root, relativePath, fragment string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
	}
	if strings.Contains(string(content), fragment) {
		t.Fatalf("%s = %q, do not want fragment %q", relativePath, string(content), fragment)
	}
}
