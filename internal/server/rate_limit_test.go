package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/inkyvoxel/go-spark/internal/paths"
)

func TestInMemoryRateLimiterAllowWithinLimitThenDeny(t *testing.T) {
	limiter := newFixedWindowRateLimiter()
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
	limiter := newFixedWindowRateLimiter()
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
	limiter := newFixedWindowRateLimiter()
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

func TestInMemoryRateLimiterDeniesNewKeysWhenStoreIsFull(t *testing.T) {
	limiter := newFixedWindowRateLimiter()
	policy := RateLimitPolicy{MaxRequests: 2, Window: 2 * time.Minute}
	now := time.Unix(400, 0)

	for i := 0; i < maxRateLimitEntries; i++ {
		key := "key-" + strconv.Itoa(i)
		limiter.entries[key] = rateLimitEntry{Count: 1, ResetAt: now.Add(time.Minute)}
	}

	allowed, retryAfter := limiter.Allow("new-key", policy, now)
	if allowed {
		t.Fatal("Allow() allowed new key with full store, want denied")
	}
	if retryAfter != policy.Window {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, policy.Window)
	}
}

func TestInMemoryRateLimiterStillTracksExistingKeyWhenStoreIsFull(t *testing.T) {
	limiter := newFixedWindowRateLimiter()
	policy := RateLimitPolicy{MaxRequests: 2, Window: time.Minute}
	now := time.Unix(500, 0)
	bucketKey := "existing"

	limiter.entries[bucketKey] = rateLimitEntry{Count: 1, ResetAt: now.Add(policy.Window)}
	for i := 0; i < maxRateLimitEntries-1; i++ {
		key := "filler-" + strconv.Itoa(i)
		limiter.entries[key] = rateLimitEntry{Count: 1, ResetAt: now.Add(policy.Window)}
	}

	allowed, retryAfter := limiter.Allow(bucketKey, policy, now.Add(5*time.Second))
	if !allowed || retryAfter != 0 {
		t.Fatalf("Allow(existing) = (%v, %v), want (true, 0)", allowed, retryAfter)
	}

	allowed, retryAfter = limiter.Allow(bucketKey, policy, now.Add(10*time.Second))
	if allowed {
		t.Fatal("Allow(existing) over limit = allowed, want denied")
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want > 0", retryAfter)
	}
}

func TestInMemoryRateLimiterResetsExpiredKeyWhenStoreIsFull(t *testing.T) {
	limiter := newFixedWindowRateLimiter()
	policy := RateLimitPolicy{MaxRequests: 2, Window: time.Minute}
	now := time.Unix(600, 0)
	bucketKey := "existing"

	limiter.entries[bucketKey] = rateLimitEntry{Count: 2, ResetAt: now.Add(-time.Second)}
	for i := range maxRateLimitEntries - 1 {
		key := "filler-" + strconv.Itoa(i)
		limiter.entries[key] = rateLimitEntry{Count: 1, ResetAt: now.Add(time.Minute)}
	}

	allowed, retryAfter := limiter.Allow(bucketKey, policy, now)
	if !allowed || retryAfter != 0 {
		t.Fatalf("Allow(expired key, full store) = (%v, %v), want (true, 0): expired keys reset in place and should not be denied", allowed, retryAfter)
	}
}

func TestRequestIPNoTrustedProxies(t *testing.T) {
	srv := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("X-Real-IP", "9.9.9.9")

	if got := srv.requestIP(req); got != "1.2.3.4" {
		t.Fatalf("requestIP() = %q, want %q (should ignore header when no trusted proxies)", got, "1.2.3.4")
	}
}

func TestRequestIPTrustedProxyReadsXRealIP(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "127.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	req.Header.Set("X-Real-IP", "9.9.9.9")

	if got := srv.requestIP(req); got != "9.9.9.9" {
		t.Fatalf("requestIP() = %q, want %q", got, "9.9.9.9")
	}
}

func TestRequestIPTrustedProxyFallsBackToXForwardedFor(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "127.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "5.5.5.5, 10.0.0.1")

	if got := srv.requestIP(req); got != "5.5.5.5" {
		t.Fatalf("requestIP() = %q, want leftmost X-Forwarded-For entry %q", got, "5.5.5.5")
	}
}

func TestRequestIPTrustedProxyCIDR(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "10.0.0.0/8")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:5678"
	req.Header.Set("X-Real-IP", "203.0.113.1")

	if got := srv.requestIP(req); got != "203.0.113.1" {
		t.Fatalf("requestIP() = %q, want %q", got, "203.0.113.1")
	}
}

func TestRequestIPUntrustedProxyIgnoresHeader(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "127.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("X-Real-IP", "9.9.9.9")

	if got := srv.requestIP(req); got != "1.2.3.4" {
		t.Fatalf("requestIP() = %q, want %q (untrusted proxy, should use RemoteAddr)", got, "1.2.3.4")
	}
}

func mustServerWithTrustedProxies(t *testing.T, proxies ...string) *Server {
	t.Helper()
	parsed, err := parseTrustedProxies(proxies)
	if err != nil {
		t.Fatalf("parseTrustedProxies() error = %v", err)
	}
	return &Server{trustedProxies: parsed}
}

func TestParseTrustedProxiesTrimsWhitespaceEntries(t *testing.T) {
	parsed, err := parseTrustedProxies([]string{" 127.0.0.1 ", "\t10.0.0.0/8\t"})
	if err != nil {
		t.Fatalf("parseTrustedProxies() error = %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("len(parsed) = %d, want 2", len(parsed))
	}
}

func TestParseTrustedProxiesTrimsWhitespaceAroundIP(t *testing.T) {
	parsed, err := parseTrustedProxies([]string{"  127.0.0.1  "})
	if err != nil {
		t.Fatalf("parseTrustedProxies() error = %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("len(parsed) = %d, want 1", len(parsed))
	}
	if !parsed[0].Contains(net.ParseIP("127.0.0.1")) {
		t.Fatal("parsed trusted proxy does not contain 127.0.0.1")
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
		user: verifiedRouteUser(),
	}
	srv := newAuthRouteTestServer(t, auth)
	srv.rateLimitPolicies = RateLimitPolicies{
		Login:                     RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		Register:                  RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ForgotPassword:            RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ResetPassword:             RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		PublicResendVerification:  RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		AccountResendVerification: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ChangePassword:            RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ChangeEmail:               RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		RevokeSession:             RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		RevokeOtherSessions:       RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
	}
	routes := srv.Routes()

	tests := []struct {
		name         string
		path         string
		form         url.Values
		sessionToken string
		resetToken   string
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
			name: "reset-password",
			path: paths.ResetPassword,
			form: url.Values{
				"new_password":     []string{"new-password"},
				"confirm_password": []string{"new-password"},
			},
			resetToken: "reset-token",
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
		{
			name: "change-password",
			path: paths.ChangePassword,
			form: url.Values{
				"current_password": []string{"old-password"},
				"new_password":     []string{"new-password"},
				"confirm_password": []string{"new-password"},
			},
			sessionToken: "session-token",
		},
		{
			name: "change-email",
			path: paths.ChangeEmail,
			form: url.Values{
				"email":            []string{"new@example.com"},
				"current_password": []string{"password"},
			},
			sessionToken: "session-token",
		},
		{
			name: "revoke-session",
			path: paths.AccountSessionsRevoke,
			form: url.Values{
				"session_id": []string{"2"},
			},
			sessionToken: "session-token",
		},
		{
			name:         "revoke-other-sessions",
			path:         paths.AccountSessionsRevokeOthers,
			form:         url.Values{},
			sessionToken: "session-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := postFormWithCSRF(t, srv, routes, tt.path, tt.form, tt.sessionToken, tt.resetToken)
			if first.Code == http.StatusTooManyRequests {
				t.Fatalf("first status = %d, want non-429", first.Code)
			}

			second := postFormWithCSRF(t, srv, routes, tt.path, tt.form, tt.sessionToken, tt.resetToken)
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

	first := postFormWithCSRF(t, srv, routes, paths.ForgotPassword, url.Values{"email": []string{"a@example.com"}}, "", "")
	if first.Code == http.StatusTooManyRequests {
		t.Fatalf("first status = %d, want non-429", first.Code)
	}

	second := postFormWithCSRF(t, srv, routes, paths.ForgotPassword, url.Values{"email": []string{"a@example.com"}}, "", "")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}

	third := postFormWithCSRF(t, srv, routes, paths.ForgotPassword, url.Values{"email": []string{"b@example.com"}}, "", "")
	if third.Code == http.StatusTooManyRequests {
		t.Fatalf("third status = %d, want non-429 for different email", third.Code)
	}
}

func TestRouteRateLimitKeyingByIPAndResetTokenCookie(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)
	srv.rateLimitPolicies = RateLimitPolicies{
		ResetPassword: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
	}
	routes := srv.Routes()

	first := postFormWithCSRF(t, srv, routes, paths.ResetPassword, url.Values{
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
	}, "", "token-a")
	if first.Code == http.StatusTooManyRequests {
		t.Fatalf("first status = %d, want non-429", first.Code)
	}

	second := postFormWithCSRF(t, srv, routes, paths.ResetPassword, url.Values{
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
	}, "", "token-a")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}

	third := postFormWithCSRF(t, srv, routes, paths.ResetPassword, url.Values{
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
	}, "", "token-b")
	if third.Code == http.StatusTooManyRequests {
		t.Fatalf("third status = %d, want non-429 for different reset token", third.Code)
	}
}

func TestRouteRateLimitKeyingByIPAndUser(t *testing.T) {
	auth := &fakeAuthLookup{user: verifiedRouteUser()}
	srv := newAuthRouteTestServer(t, auth)
	srv.rateLimitPolicies = RateLimitPolicies{
		AccountResendVerification: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ChangePassword:            RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ChangeEmail:               RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		RevokeSession:             RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		RevokeOtherSessions:       RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
	}
	routes := srv.Routes()

	changePasswordForm := url.Values{
		"current_password": []string{"old-password"},
		"new_password":     []string{"new-password"},
		"confirm_password": []string{"new-password"},
	}
	first := postFormWithCSRF(t, srv, routes, paths.ChangePassword, changePasswordForm, "session-token", "")
	if first.Code == http.StatusTooManyRequests {
		t.Fatalf("first status = %d, want non-429", first.Code)
	}

	auth.user = verifiedRouteUser()
	auth.user.ID = 2
	auth.user.Email = "user2@example.com"
	second := postFormWithCSRF(t, srv, routes, paths.ChangePassword, changePasswordForm, "session-token", "")
	if second.Code == http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want non-429 for different user", second.Code)
	}

	auth.user = verifiedRouteUser()
	third := postFormWithCSRF(t, srv, routes, paths.ChangePassword, changePasswordForm, "session-token", "")
	if third.Code != http.StatusTooManyRequests {
		t.Fatalf("third status = %d, want %d for original user", third.Code, http.StatusTooManyRequests)
	}
}

func postFormWithCSRF(t *testing.T, srv *Server, routes http.Handler, path string, form url.Values, sessionToken, resetToken string) *httptest.ResponseRecorder {
	t.Helper()

	if form == nil {
		form = url.Values{}
	}
	form.Set(csrfFieldName, "csrf")

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	}
	if resetToken != "" {
		req.AddCookie(&http.Cookie{Name: resetCookieName, Value: resetToken})
	}
	addCSRFCookieAndHeader(t, srv, req)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	return rec
}
