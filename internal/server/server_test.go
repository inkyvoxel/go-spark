package server

import (
	"database/sql"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestRoutesSetSecurityHeaders(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
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

func testServer(t *testing.T) *Server {
	t.Helper()

	return &Server{
		db:     testDB(t),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: testTemplates(t, map[string]string{
			"home.html": `<h1>{{ .Title }}</h1>`,
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

		templates[name] = template.Must(template.New("layout.html").Parse(`
			{{ define "layout.html" }}
				<!doctype html>
				<title>{{ .Title }}</title>
				{{ template "content" . }}
			{{ end }}
			` + parsedContent + `
		`))
	}

	return templates
}
