package server

import (
	"database/sql"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/paths"

	_ "modernc.org/sqlite"
)

func TestNewRequiresAuthService(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New() did not panic")
		}
	}()

	New(Options{DB: testDB(t)})
}

func TestRoutesHome(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Home, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Go Spark") {
		t.Fatalf("body = %q, want it to contain %q", rec.Body.String(), "Go Spark")
	}
}

func TestRoutesHealthz(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Healthz, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok\n" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok\n")
	}
}

func TestRoutesHealthzReturnsUnavailableWhenDatabaseIsClosed(t *testing.T) {
	srv := testServer(t)
	if err := srv.db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, paths.Healthz, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestRoutesSetSecurityHeaders(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Home, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Security-Policy"); got != cspHeaderValue {
		t.Fatalf("Content-Security-Policy = %q, want %q", got, cspHeaderValue)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want %q", got, "no-referrer")
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want %q", got, "DENY")
	}
	if got := rec.Header().Get("Permissions-Policy"); got != "geolocation=(), microphone=(), camera=()" {
		t.Fatalf("Permissions-Policy = %q, want expected policy", got)
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q, want empty for insecure requests", got)
	}
}

func TestRoutesSetHSTSWhenSecureCookiesEnabled(t *testing.T) {
	srv := testServer(t)
	srv.cookieSecure = true

	req := httptest.NewRequest(http.MethodGet, paths.Home, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("Strict-Transport-Security = %q, want %q", got, "max-age=31536000")
	}
}

func TestRenderReturnsInternalServerErrorForTemplateError(t *testing.T) {
	srv := testServer(t)

	rec := httptest.NewRecorder()
	srv.render(rec, "missing.html", templateData{Title: "Missing"})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if rec.Body.String() != http.StatusText(http.StatusInternalServerError)+"\n" {
		t.Fatalf("body = %q, want internal server error", rec.Body.String())
	}
}

func TestParseTemplatesIncludesBreadcrumbPartial(t *testing.T) {
	chdirProjectRoot(t)

	templates, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	var body strings.Builder
	err = templates[templateChangePassword].ExecuteTemplate(&body, templateLayout, templateData{
		Title:  "Change Password",
		Routes: paths.TemplateRoutes,
		Breadcrumbs: []breadcrumbItem{
			{Label: "Account", URL: paths.Account},
			{Label: "Change password", Current: true},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	rendered := body.String()
	for _, want := range []string{`aria-label="breadcrumb"`, `href="/account"`, `aria-current="page"`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered template = %q, want %q", rendered, want)
		}
	}
}

func chdirProjectRoot(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("chdir project root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})
}

func testServer(t *testing.T) *Server {
	t.Helper()

	return &Server{
		db:     testDB(t),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: testTemplates(t, map[string]string{
			templateHome: `<h1>{{ .Title }}</h1>`,
		}),
	}
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func testTemplates(t *testing.T, pages map[string]string) map[string]*template.Template {
	t.Helper()

	templates := make(map[string]*template.Template, len(pages))
	for name, content := range pages {
		parsedContent := `{{ define "content" }}` + content + `{{ end }}`
		if strings.Contains(content, `{{ define "content"`) {
			parsedContent = content
		}

		templates[name] = template.Must(template.New(templateLayout).Parse(
			`{{ define "` + templateLayout + `" }}
				<!doctype html>
				<title>{{ .Title }}</title>
				{{ template "content" . }}
			{{ end }}` + parsedContent,
		))
	}

	return templates
}
