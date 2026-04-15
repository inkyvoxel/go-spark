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
		templates[name] = template.Must(template.New("layout.html").Parse(`
			{{ define "layout.html" }}
				<!doctype html>
				<title>{{ .Title }}</title>
				{{ template "content" . }}
			{{ end }}
			{{ define "content" }}` + content + `{{ end }}
		`))
	}

	return templates
}
