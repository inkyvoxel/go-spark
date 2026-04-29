package generator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type generatedRouteExpectation struct {
	Path       string
	WantStatus int
}

func TestGeneratedProjectsSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping generated-project smoke tests in short mode")
	}

	tests := []struct {
		name            string
		features        []string
		routes          []generatedRouteExpectation
		fileContains    map[string][]string
		fileNotContains map[string][]string
		filesExist      []string
		filesNotExist   []string
	}{
		{
			name:     "core-minimal",
			features: []string{FeatureCore},
			routes: []generatedRouteExpectation{
				{Path: "/", WantStatus: 200},
				{Path: "/healthz", WantStatus: 200},
				{Path: "/readyz", WantStatus: 200},
				{Path: "/login", WantStatus: 404},
				{Path: "/account/forgot-password", WantStatus: 404},
				{Path: "/account/resend-verification", WantStatus: 404},
			},
			fileContains: map[string][]string{
				"internal/features/features.go": {
					"Auth:              false",
					"PasswordReset:     false",
					"EmailOutbox:       false",
					"EmailVerification: false",
					"EmailChange:       false",
					"Worker:            false",
					"Cleanup:           false",
				},
			},
			filesExist: []string{
				"migrations/00001_empty_schema.sql",
				"templates/home.html",
				"internal/server/server.go",
			},
			filesNotExist: []string{
				"docs/email.md",
				"docs/jobs.md",
				"templates/account/login.html",
				"templates/account/forgot_password.html",
				"templates/account/resend_verification.html",
				"migrations/00001_auth_schema.sql",
			},
		},
		{
			name:     "auth-without-verification",
			features: []string{FeatureAuth},
			routes: []generatedRouteExpectation{
				{Path: "/", WantStatus: 200},
				{Path: "/login", WantStatus: 200},
				{Path: "/register", WantStatus: 200},
				{Path: "/account", WantStatus: 303},
				{Path: "/account/forgot-password", WantStatus: 404},
				{Path: "/account/resend-verification", WantStatus: 404},
			},
			fileContains: map[string][]string{
				"internal/features/features.go": {
					"Auth:              true",
					"PasswordReset:     false",
					"EmailOutbox:       true",
					"EmailVerification: false",
					"EmailChange:       false",
					"Worker:            false",
					"Cleanup:           false",
				},
			},
			filesExist: []string{
				"docs/email.md",
				"templates/account/login.html",
				"templates/account/register.html",
				"migrations/00001_auth_schema.sql",
			},
			filesNotExist: []string{
				"docs/jobs.md",
				"templates/account/forgot_password.html",
				"templates/account/resend_verification.html",
				"templates/account/change_email.html",
				"migrations/00002_email_verification_schema.sql",
			},
		},
		{
			name:     "auth-password-reset",
			features: []string{FeaturePasswordReset},
			routes: []generatedRouteExpectation{
				{Path: "/", WantStatus: 200},
				{Path: "/login", WantStatus: 200},
				{Path: "/register", WantStatus: 200},
				{Path: "/account/forgot-password", WantStatus: 200},
				{Path: "/account/reset-password", WantStatus: 400},
				{Path: "/account/resend-verification", WantStatus: 404},
			},
			fileContains: map[string][]string{
				"internal/features/features.go": {
					"Auth:              true",
					"PasswordReset:     true",
					"EmailOutbox:       true",
					"EmailVerification: false",
					"EmailChange:       false",
					"Worker:            false",
					"Cleanup:           false",
				},
			},
			filesExist: []string{
				"docs/email.md",
				"templates/account/login.html",
				"templates/account/forgot_password.html",
				"templates/account/reset_password.html",
				"migrations/00003_password_reset_schema.sql",
			},
			filesNotExist: []string{
				"docs/jobs.md",
				"templates/account/resend_verification.html",
				"templates/account/change_email.html",
				"migrations/00002_email_verification_schema.sql",
			},
		},
		{
			name:     "full-feature-output",
			features: []string{FeatureAll},
			routes: []generatedRouteExpectation{
				{Path: "/", WantStatus: 200},
				{Path: "/login", WantStatus: 200},
				{Path: "/register", WantStatus: 200},
				{Path: "/account/forgot-password", WantStatus: 200},
				{Path: "/account/resend-verification", WantStatus: 200},
				{Path: "/account/verify-email", WantStatus: 303},
				{Path: "/account/change-email", WantStatus: 303},
			},
			fileContains: map[string][]string{
				"internal/features/features.go": {
					"Auth:              true",
					"PasswordReset:     true",
					"EmailOutbox:       true",
					"EmailVerification: true",
					"EmailChange:       true",
					"Worker:            true",
					"Cleanup:           true",
				},
			},
			filesExist: []string{
				"docs/email.md",
				"docs/jobs.md",
				"templates/account/login.html",
				"templates/account/forgot_password.html",
				"templates/account/resend_verification.html",
				"templates/account/change_email.html",
				"migrations/00001_auth_schema.sql",
				"migrations/00002_email_verification_schema.sql",
				"migrations/00003_password_reset_schema.sql",
				"migrations/00004_email_change_schema.sql",
				"migrations/00005_email_outbox_schema.sql",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "app")
			gen := New()
			gen.Stdout = nil

			_, err := gen.NewProject(ProjectOptions{
				TargetPath:   target,
				ProjectName:  "Generated " + tt.name,
				ModulePath:   "github.com/example/generated-" + strings.ReplaceAll(tt.name, "_", "-"),
				DatabasePath: "./data/app.db",
				EmailFrom:    "Generated <hello@example.com>",
				Features:     tt.features,
				Yes:          true,
			})
			if err != nil {
				t.Fatalf("NewProject() error = %v", err)
			}

			for path, fragments := range tt.fileContains {
				for _, fragment := range fragments {
					assertGeneratedFileContains(t, target, path, fragment)
				}
			}
			for path, fragments := range tt.fileNotContains {
				for _, fragment := range fragments {
					assertGeneratedFileNotContains(t, target, path, fragment)
				}
			}
			for _, relativePath := range tt.filesExist {
				assertGeneratedFileExists(t, target, relativePath)
			}
			for _, relativePath := range tt.filesNotExist {
				assertGeneratedFileNotExists(t, target, relativePath)
			}
			for _, maintainerFile := range []string{"CONTRIBUTING.md", "CHANGELOG.md", "docs/todo.md"} {
				assertGeneratedFileNotExists(t, target, maintainerFile)
			}
			assertGeneratedFileContains(t, target, "README.md", "Created with [go-spark](https://github.com/inkyvoxel/go-spark).")

			writeGeneratedSmokeTest(t, target, tt.routes)
			runGeneratedGoTest(t, target)
		})
	}
}

func writeGeneratedSmokeTest(t *testing.T, target string, routes []generatedRouteExpectation) {
	t.Helper()

	var routeLines []string
	for _, route := range routes {
		routeLines = append(routeLines, fmt.Sprintf("\t\t{path: %q, wantStatus: %d},", route.Path, route.WantStatus))
	}

	body := fmt.Sprintf(`package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	bootstrap "github.com/example/generated/internal/app"
	"github.com/example/generated/internal/config"
	"github.com/example/generated/internal/services"
)

type routeExpectation struct {
	path       string
	wantStatus int
}

func TestGeneratedProjectSmoke(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "app.db")
	t.Setenv("DATABASE_PATH", databasePath)
	t.Setenv("APP_ADDR", "127.0.0.1:18080")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_COOKIE_SECURE", "false")
	t.Setenv("EMAIL_PROVIDER", "log")
	t.Setenv("EMAIL_LOG_BODY", "false")

	if err := runMigrate("up"); err != nil {
		t.Fatalf("runMigrate(up) error = %%v", err)
	}

	cfg, err := config.FromEnvWithProcess(services.DefaultPasswordMinLength, config.ProcessWeb)
	if err != nil {
		t.Fatalf("FromEnvWithProcess() error = %%v", err)
	}

	runtime, err := bootstrap.Build(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("Build() error = %%v", err)
	}
	defer runtime.Close()

	handler := runtime.HTTPServer.Handler
	checks := []routeExpectation{
%s
	}

	for _, check := range checks {
		req := httptest.NewRequest(http.MethodGet, check.path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != check.wantStatus {
			t.Fatalf("GET %%s status = %%d, want %%d; body=%%q", check.path, rec.Code, check.wantStatus, rec.Body.String())
		}
	}
}
`, strings.Join(routeLines, "\n"))

	body = strings.ReplaceAll(body, "github.com/example/generated", moduleImportPath(t, target))

	testPath := filepath.Join(target, "cmd", "app", "generated_smoke_test.go")
	if err := os.WriteFile(testPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", testPath, err)
	}
}

func moduleImportPath(t *testing.T, target string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(target, "go.mod"))
	if err != nil {
		t.Fatalf("ReadFile(go.mod) error = %v", err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	t.Fatal("module path not found in go.mod")
	return ""
}

func runGeneratedGoTest(t *testing.T, target string) {
	t.Helper()
	runGeneratedCommand(t, target, "go", "test", "./...", "-run", "TestGeneratedProjectSmoke", "-count=1")
}

func runGeneratedCommand(t *testing.T, target string, name string, args ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = target
	cmd.Env = append(
		os.Environ(),
		"GOCACHE="+filepath.Join(t.TempDir(), "go-build-cache"),
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("%s %s timed out\n%s", name, strings.Join(args, " "), output)
	}
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
}

func assertGeneratedFileExists(t *testing.T, root, relativePath string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, relativePath)); err != nil {
		t.Fatalf("Stat(%q) error = %v", relativePath, err)
	}
}
