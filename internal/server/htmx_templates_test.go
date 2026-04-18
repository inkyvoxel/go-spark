package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHTMXFormTemplatesIncludeLoadingAffordances(t *testing.T) {
	templateFiles := []string{
		"account/login.html",
		"account/register.html",
		"account/forgot_password.html",
		"account/resend_verification.html",
		"account/reset_password.html",
		"account/change_password.html",
		"account/verify_email.html",
	}

	for _, name := range templateFiles {
		content := readProjectFile(t, "templates", name)
		if !strings.Contains(content, `hx-disabled-elt="button[type='submit']"`) {
			t.Fatalf("%s missing hx-disabled-elt submit binding", name)
		}
		if !strings.Contains(content, `hx-on::before-request="this.querySelector('button[type=submit]').setAttribute('aria-busy','true')"`) {
			t.Fatalf("%s missing before-request aria-busy hook", name)
		}
		if !strings.Contains(content, `hx-on::after-request="this.querySelector('button[type=submit]').removeAttribute('aria-busy')"`) {
			t.Fatalf("%s missing after-request aria-busy hook", name)
		}
	}
}

func TestLayoutDoesNotDefineCustomHTMXLoadingIndicatorTemplate(t *testing.T) {
	layout := readProjectFile(t, "templates", "layout.html")
	if strings.Contains(layout, `define "htmx_form_loading_indicator"`) {
		t.Fatal("layout should not define custom htmx_form_loading_indicator template")
	}
}

func TestStylesDoNotIncludeCustomHTMXLoadingIndicatorRules(t *testing.T) {
	styles := readProjectFile(t, "static", "styles.css")
	if strings.Contains(styles, ".htmx-form-indicator") {
		t.Fatal("styles should not include .htmx-form-indicator rule")
	}
	if strings.Contains(styles, ".htmx-request .htmx-form-indicator") {
		t.Fatal("styles should not include custom indicator loading-state rule")
	}
}

func readProjectFile(t *testing.T, parts ...string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	fullPath := filepath.Join(append([]string{projectRoot}, parts...)...)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}

	return string(content)
}
