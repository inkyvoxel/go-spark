package server

import (
	"crypto/tls"
	"database/sql"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/features"
	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"

	_ "modernc.org/sqlite"
)

func TestNewRequiresAuthService(t *testing.T) {
	_, err := New(Options{DB: testDB(t)})
	if err == nil {
		t.Fatal("New() with no auth service should return an error")
	}
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
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body = %q, want %q", got, "ok")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestRoutesReadyz(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Readyz, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body = %q, want %q", got, "ok")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestRoutesReadyzReturnsServiceUnavailableWhenNotReady(t *testing.T) {
	srv := testServer(t)
	srv.db = nil

	req := httptest.NewRequest(http.MethodGet, paths.Readyz, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Body.String(); got != "not ready" {
		t.Fatalf("body = %q, want %q", got, "not ready")
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "error") {
		t.Fatalf("body = %q, want no diagnostic details", rec.Body.String())
	}
}

func TestRoutesUnknownGetRendersCustomNotFound(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if !strings.Contains(rec.Body.String(), "Page not found") {
		t.Fatalf("body = %q, want custom 404 content", rec.Body.String())
	}
	if strings.EqualFold(strings.TrimSpace(rec.Body.String()), http.StatusText(http.StatusNotFound)) {
		t.Fatalf("body = %q, want custom html response instead of default text", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != cspHeaderValue {
		t.Fatalf("Content-Security-Policy = %q, want %q", got, cspHeaderValue)
	}
}

func TestRoutesUnknownHeadReturnsNotFound(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodHead, "/does-not-exist", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != cspHeaderValue {
		t.Fatalf("Content-Security-Policy = %q, want %q", got, cspHeaderValue)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/html; charset=utf-8")
	}
}

func TestRoutesKnownPathWrongMethodReturnsMethodNotAllowed(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Logout, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", allow, http.MethodPost)
	}
}

func TestRoutesStaticDirectoryListingIsNotServed(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.StaticPrefix, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("Cache-Control = %q, want empty", got)
	}
}

func TestRoutesStaticFileIsServed(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.StaticStyles, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheControlPublic {
		t.Fatalf("Cache-Control = %q, want %q", got, cacheControlPublic)
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

func TestRoutesSetHSTSWhenRequestIsTLS(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Home, nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("Strict-Transport-Security = %q, want %q", got, "max-age=31536000")
	}
}

func TestRoutesSetNoStoreCacheHeadersForAuthSensitiveGet(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Account, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	assertNoStoreCacheHeaders(t, rec)
}

func TestRoutesDoNotSetNoStoreCacheHeadersForPublicGet(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodGet, paths.Home, nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("Cache-Control = %q, want empty", got)
	}
	if got := rec.Header().Get("Pragma"); got != "" {
		t.Fatalf("Pragma = %q, want empty", got)
	}
	if got := rec.Header().Get("Expires"); got != "" {
		t.Fatalf("Expires = %q, want empty", got)
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
	templates, err := parseTemplates(features.Enabled)
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

func TestParseTemplatesWorksOutsideProjectRoot(t *testing.T) {
	t.Helper()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	templates, err := parseTemplates(features.Enabled)
	if err != nil {
		t.Fatalf("parseTemplates() error = %v", err)
	}

	if _, ok := templates[templateHome]; !ok {
		t.Fatalf("expected template %q to be parsed", templateHome)
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()

	return &Server{
		db:                      testDB(t),
		emailVerificationPolicy: services.DefaultEmailVerificationPolicy(),
		logger:                  slog.New(slog.NewTextHandler(io.Discard, nil)),
		csrfSigningKey:          []byte("test-csrf-signing-key"),
		templates: testTemplates(t, map[string]string{
			templateHome:     `<h1>{{ .Title }}</h1>`,
			templateNotFound: `<h1>Page not found</h1>`,
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

func assertNoStoreCacheHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get("Cache-Control"); got != cacheControlNoStore {
		t.Fatalf("Cache-Control = %q, want %q", got, cacheControlNoStore)
	}
	if got := rec.Header().Get("Pragma"); got != pragmaNoCache {
		t.Fatalf("Pragma = %q, want %q", got, pragmaNoCache)
	}
	if got := rec.Header().Get("Expires"); got != expiresImmediately {
		t.Fatalf("Expires = %q, want %q", got, expiresImmediately)
	}
}
