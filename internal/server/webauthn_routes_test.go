package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/paths"
)

func TestRoutesPasskeyLoginBeginRejectsCrossOrigin(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{passkeysEnabled: true})

	req := httptest.NewRequest(http.MethodPost, paths.LoginPasskeyBegin, nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (cross-origin)", rec.Code, http.StatusForbidden)
	}
}

func TestRoutesPasskeyLoginBeginReturnsJSON(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{passkeysEnabled: true})

	req := httptest.NewRequest(http.MethodPost, paths.LoginPasskeyBegin, nil)
	setSameOriginFetch(req)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	// The ceremony session data is stored in a signed, path-scoped cookie.
	cookie := cookieFromRecorder(t, rec, passkeyLoginCookieName)
	if cookie.Path != passkeyLoginCookiePath {
		t.Fatalf("cookie path = %q, want %q", cookie.Path, passkeyLoginCookiePath)
	}
	if !cookie.HttpOnly {
		t.Fatal("ceremony cookie should be HttpOnly")
	}
}

func TestRoutesPasskeyLoginFinishWithoutCeremonyCookie(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{passkeysEnabled: true})

	req := httptest.NewRequest(http.MethodPost, paths.LoginPasskeyFinish, strings.NewReader("{}"))
	setSameOriginFetch(req)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (no ceremony cookie)", rec.Code, http.StatusBadRequest)
	}
}

func TestRoutesPasskeyRegisterBeginRequiresAuth(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{passkeysEnabled: true})

	req := httptest.NewRequest(http.MethodPost, paths.AccountPasskeysRegisterBegin, nil)
	setSameOriginFetch(req)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (anonymous)", rec.Code, http.StatusUnauthorized)
	}
}

func TestRoutesPasskeyEndpointsRejectGET(t *testing.T) {
	srv := newAuthRouteTestServer(t, &fakeAuthLookup{passkeysEnabled: true})

	for _, path := range []string{paths.LoginPasskeyBegin, paths.LoginPasskeyFinish, paths.AccountPasskeysRegisterBegin} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}
