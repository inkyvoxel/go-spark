package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/paths"
)

func TestInMemoryRateLimiterAllowWithinLimitThenDeny(t *testing.T) {
	limiter := newInMemoryRateLimiter()
	policy := RateLimitPolicy{MaxRequests: 2, Window: time.Minute}
	now := time.Unix(100, 0)

	allowed, retryAfter := limiter.Allow("bucket", policy, now)
	if !allowed || retryAfter != 0 {
		t.Fatalf("first Allow() = (%v, %v), want (true, 0)", allowed, retryAfter)
	}

	allowed, retryAfter = limiter.Allow("bucket", policy, now.Add(5*time.Second))
	if !allowed || retryAfter != 0 {
		t.Fatalf("second Allow() = (%v, %v), want (true, 0)", allowed, retryAfter)
	}

	allowed, retryAfter = limiter.Allow("bucket", policy, now.Add(10*time.Second))
	if allowed {
		t.Fatal("third Allow() = allowed, want denied")
	}
	if retryAfter <= 0 || retryAfter > 50*time.Second {
		t.Fatalf("retryAfter = %v, want >0 and <=50s", retryAfter)
	}
}

func TestInMemoryRateLimiterWindowReset(t *testing.T) {
	limiter := newInMemoryRateLimiter()
	policy := RateLimitPolicy{MaxRequests: 1, Window: time.Minute}
	now := time.Unix(200, 0)

	allowed, _ := limiter.Allow("bucket", policy, now)
	if !allowed {
		t.Fatal("first Allow() denied, want allowed")
	}

	allowed, _ = limiter.Allow("bucket", policy, now.Add(time.Second))
	if allowed {
		t.Fatal("second Allow() allowed, want denied")
	}

	allowed, _ = limiter.Allow("bucket", policy, now.Add(policy.Window))
	if !allowed {
		t.Fatal("Allow() after window reset denied, want allowed")
	}
}

func TestInMemoryRateLimiterCleanupRemovesExpiredEntries(t *testing.T) {
	limiter := newInMemoryRateLimiter()
	now := time.Unix(300, 0)
	limiter.entries["expired"] = rateLimitEntry{Count: 1, ResetAt: now.Add(-time.Second)}
	limiter.entries["active"] = rateLimitEntry{Count: 1, ResetAt: now.Add(time.Minute)}
	limiter.calls = cleanupEveryNRateLimitCalls - 1

	allowed, _ := limiter.Allow("new", RateLimitPolicy{MaxRequests: 1, Window: time.Minute}, now)
	if !allowed {
		t.Fatal("Allow() denied, want allowed")
	}

	if _, ok := limiter.entries["expired"]; ok {
		t.Fatal("expired entry still present after cleanup")
	}
	if _, ok := limiter.entries["active"]; !ok {
		t.Fatal("active entry removed during cleanup")
	}
}

func TestNormalizedEmailForRateLimit(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		want  string
		valid bool
	}{
		{name: "normalized", raw: "  USER@Example.com ", want: "user@example.com", valid: true},
		{name: "with display name", raw: "User <USER@Example.com>", want: "user@example.com", valid: true},
		{name: "invalid", raw: "not-an-email", valid: false},
		{name: "blank", raw: "  ", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizedEmailForRateLimit(tt.raw)
			if ok != tt.valid {
				t.Fatalf("normalizedEmailForRateLimit(%q) ok = %v, want %v", tt.raw, ok, tt.valid)
			}
			if got != tt.want {
				t.Fatalf("normalizedEmailForRateLimit(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRouteRateLimitProtectedPostRoutesReturn429AfterThreshold(t *testing.T) {
	auth := &fakeAuthLookup{
		user: db.User{ID: 1, Email: "user@example.com"},
	}
	srv := newAuthRouteTestServer(t, auth)
	srv.rateLimitPolicies = RateLimitPolicies{
		Login:                     RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		Register:                  RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ForgotPassword:            RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		PublicResendVerification:  RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		AccountResendVerification: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
	}
	routes := srv.Routes()

	tests := []struct {
		name         string
		path         string
		form         url.Values
		sessionToken string
	}{
		{
			name: "login",
			path: paths.Login,
			form: url.Values{"email": []string{"user@example.com"}, "password": []string{"password"}},
		},
		{
			name: "register",
			path: paths.Register,
			form: url.Values{"email": []string{"new@example.com"}, "password": []string{"password123"}, "confirm_password": []string{"password123"}},
		},
		{
			name: "forgot-password",
			path: paths.ForgotPassword,
			form: url.Values{"email": []string{"user@example.com"}},
		},
		{
			name: "resend-verification-public",
			path: paths.ResendVerification,
			form: url.Values{"email": []string{"user@example.com"}},
		},
		{
			name:         "resend-verification-account",
			path:         paths.VerifyEmailResend,
			form:         url.Values{},
			sessionToken: "session-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := postFormWithCSRF(routes, tt.path, tt.form, tt.sessionToken)
			if first.Code == http.StatusTooManyRequests {
				t.Fatalf("first status = %d, want non-429", first.Code)
			}

			second := postFormWithCSRF(routes, tt.path, tt.form, tt.sessionToken)
			if second.Code != http.StatusTooManyRequests {
				t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
			}
			if second.Header().Get("Retry-After") == "" {
				t.Fatal("Retry-After header missing")
			}
		})
	}
}

func TestRouteRateLimitKeyingByIPAndEmail(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)
	srv.rateLimitPolicies = RateLimitPolicies{
		ForgotPassword: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
	}
	routes := srv.Routes()

	first := postFormWithCSRF(routes, paths.ForgotPassword, url.Values{"email": []string{"a@example.com"}}, "")
	if first.Code == http.StatusTooManyRequests {
		t.Fatalf("first status = %d, want non-429", first.Code)
	}

	second := postFormWithCSRF(routes, paths.ForgotPassword, url.Values{"email": []string{"a@example.com"}}, "")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}

	third := postFormWithCSRF(routes, paths.ForgotPassword, url.Values{"email": []string{"b@example.com"}}, "")
	if third.Code == http.StatusTooManyRequests {
		t.Fatalf("third status = %d, want non-429 for different email", third.Code)
	}
}

func TestRouteRateLimitKeyingByIPAndUser(t *testing.T) {
	auth := &fakeAuthLookup{user: db.User{ID: 1, Email: "user1@example.com"}}
	srv := newAuthRouteTestServer(t, auth)
	srv.rateLimitPolicies = RateLimitPolicies{
		AccountResendVerification: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
	}
	routes := srv.Routes()

	first := postFormWithCSRF(routes, paths.VerifyEmailResend, url.Values{}, "session-token")
	if first.Code == http.StatusTooManyRequests {
		t.Fatalf("first status = %d, want non-429", first.Code)
	}

	auth.user = db.User{ID: 2, Email: "user2@example.com"}
	second := postFormWithCSRF(routes, paths.VerifyEmailResend, url.Values{}, "session-token")
	if second.Code == http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want non-429 for different user", second.Code)
	}

	auth.user = db.User{ID: 1, Email: "user1@example.com"}
	third := postFormWithCSRF(routes, paths.VerifyEmailResend, url.Values{}, "session-token")
	if third.Code != http.StatusTooManyRequests {
		t.Fatalf("third status = %d, want %d for original user", third.Code, http.StatusTooManyRequests)
	}
}

func postFormWithCSRF(routes http.Handler, path string, form url.Values, sessionToken string) *httptest.ResponseRecorder {
	if form == nil {
		form = url.Values{}
	}
	form.Set(csrfFieldName, "csrf")

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	}
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	return rec
}
