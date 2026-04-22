package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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
	if !srv.validSignedCSRFToken(cookie.Value, csrfAnonymousSessionHash, time.Now().UTC()) {
		t.Fatal("csrf cookie token is not a valid signed anonymous token")
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
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
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
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	form := url.Values{csrfFieldName: []string{token}}
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCSRFAllowsPostWithMatchingOriginHeader(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
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
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFAllowsPostWithMatchingRefererWhenOriginMissing(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	req.Header.Set("Referer", "http://localhost:8080/account/change-password")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCSRFRejectsPostWithMismatchedRefererWhenOriginMissing(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	req.Header.Set("Referer", "https://evil.example/form")
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsPostWithMalformedOriginOrReferer(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "malformed Origin",
			headers: map[string]string{
				"Origin": "://bad-origin",
			},
		},
		{
			name: "malformed Referer",
			headers: map[string]string{
				"Referer": "://bad-referer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/submit", nil)
			req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
			req.Header.Set(csrfHeaderName, token)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			rec := httptest.NewRecorder()

			srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("next handler should not run")
			})).ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestCSRFAllowsPostWhenOriginAndRefererAreMissing(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
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
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
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
	cookieToken := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())
	requestToken := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: cookieToken})
	req.Header.Set(csrfHeaderName, requestToken)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsPostWithTamperedSignature(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())
	tampered := token[:len(token)-1] + "A"

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tampered})
	req.Header.Set(csrfHeaderName, tampered)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsExpiredToken(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC().Add(-2*csrfCookieTTL))

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsAnonymousTokenAfterSessionChange(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	anonToken := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: anonToken})
	req.Header.Set(csrfHeaderName, anonToken)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFRejectsSessionBoundTokenAfterLogout(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	sessionToken := "session-token"
	authToken := mustSignedCSRFToken(t, srv, csrfSessionHash(sessionToken), time.Now().UTC())

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: authToken})
	req.Header.Set(csrfHeaderName, authToken)
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFSafeRequestRotatesInvalidCookieToken(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "legacy-invalid-token"})
	rec := httptest.NewRecorder()

	srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	cookie := cookieFromRecorder(t, rec, csrfCookieName)
	if cookie.Value == "legacy-invalid-token" {
		t.Fatal("csrf token was not rotated")
	}
}

func TestCSRFRejectsOversizedFormBody(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)
	token := mustSignedCSRFToken(t, srv, csrfAnonymousSessionHash, time.Now().UTC())

	overSizedPayload := strings.Repeat("a", maxRequestBodyBytes+1024)
	form := url.Values{
		csrfFieldName: []string{token},
		"payload":     []string{overSizedPayload},
	}
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.limitRequestBody(srv.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCSRFSafeMethodsIgnoreOriginAndRefererValidation(t *testing.T) {
	srv := newAuthMiddlewareTestServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Referer", "https://evil.example/landing")
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

func mustSignedCSRFToken(t *testing.T, srv *Server, sessionHash string, now time.Time) string {
	t.Helper()

	token, err := srv.newSignedCSRFToken(sessionHash, now)
	if err != nil {
		t.Fatalf("newSignedCSRFToken() error = %v", err)
	}
	return token
}
