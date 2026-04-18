package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRFSetsTokenCookieOnSafeRequest(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := csrfToken(r.Context()); token == "" {
			t.Fatal("csrfToken() = empty, want token")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	cookie := cookieFromRecorder(t, rec, csrfCookieName)
	if cookie.Value == "" {
		t.Fatal("csrf cookie value is empty")
	}
	if !cookie.HttpOnly {
		t.Fatal("csrf cookie HttpOnly = false, want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("csrf cookie SameSite = %v, want %v", cookie.SameSite, http.SameSiteLaxMode)
	}
}

func TestCSRFSetsSecureCookieWhenConfigured(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	srv.cookieSecure = true

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	cookie := cookieFromRecorder(t, rec, csrfCookieName)
	if !cookie.Secure {
		t.Fatal("csrf cookie Secure = false, want true")
	}
}

func TestCSRFAllowsPostWithHeaderToken(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	req.Header.Set(csrfHeaderName, "token")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCSRFAllowsPostWithFormToken(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	form := url.Values{csrfFieldName: []string{"token"}}
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCSRFRejectsPostWithoutToken(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsPostWithMismatchedToken(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	req.Header.Set(csrfHeaderName, "different")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsOversizedFormBody(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	oversizedPayload := strings.Repeat("a", maxRequestBodyBytes+1024)
	form := url.Values{
		csrfFieldName: []string{"token"},
		"payload":     []string{oversizedPayload},
	}
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	srv.limitRequestBody(srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
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
