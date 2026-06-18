package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// CSRF defense is http.CrossOriginProtection (Sec-Fetch-Site with an
// Origin-vs-Host fallback) plus the SameSite=Lax session cookie. These tests
// exercise the middleware's allow/deny decisions for unsafe methods.

func TestCSRFAllowsPostWithMatchingOriginHeader(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCSRFRejectsPostWithMismatchedOriginHeader(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsPostWithNullOriginHeader(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Origin", "null")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsPostWithMalformedOrigin(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Origin", "://bad-origin")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// Browsers send Sec-Fetch-Site on every request since 2023; it is the primary
// signal http.CrossOriginProtection uses.
func TestCSRFRejectsPostWithCrossSiteFetchMetadata(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFAllowsPostWithSameOriginFetchMetadata(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// Non-browser clients (curl, server-to-server) send neither Origin nor
// Sec-Fetch-Site; CrossOriginProtection allows these since there is no
// cross-origin signal to act on.
func TestCSRFAllowsPostWhenOriginAndFetchMetadataAreMissing(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCSRFSafeMethodsSkipCrossOriginChecks(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestIsUnsafeMethod(t *testing.T) {
	safe := []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace}
	for _, method := range safe {
		if isUnsafeMethod(method) {
			t.Fatalf("isUnsafeMethod(%q) = true, want false", method)
		}
	}

	unsafe := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range unsafe {
		if !isUnsafeMethod(method) {
			t.Fatalf("isUnsafeMethod(%q) = false, want true", method)
		}
	}
}
