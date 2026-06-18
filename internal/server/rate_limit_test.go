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
	"github.com/inkyvoxel/go-spark/internal/ratelimit"
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

func TestRequestIPIgnoresXRealIP(t *testing.T) {
	// X-Real-IP is not stripped or set by Caddy, so a client-supplied value
	// would pass through a trusted proxy untouched. It must never be trusted.
	srv := mustServerWithTrustedProxies(t, "127.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	req.Header.Set("X-Real-IP", "9.9.9.9")

	if got := srv.requestIP(req); got != "127.0.0.1" {
		t.Fatalf("requestIP() = %q, want %q (X-Real-IP is spoofable and must be ignored)", got, "127.0.0.1")
	}
}

func TestRequestIPTrustedProxyReadsXForwardedFor(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "127.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "5.5.5.5")

	if got := srv.requestIP(req); got != "5.5.5.5" {
		t.Fatalf("requestIP() = %q, want %q", got, "5.5.5.5")
	}
}

func TestRequestIPUsesRightmostUntrustedForwardedForEntry(t *testing.T) {
	// A client can send its own X-Forwarded-For header, which proxies append
	// to. Only the rightmost untrusted hop is proxy-reported and trustworthy.
	srv := mustServerWithTrustedProxies(t, "127.0.0.1", "10.0.0.0/8")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 5.5.5.5, 10.0.0.1")

	if got := srv.requestIP(req); got != "5.5.5.5" {
		t.Fatalf("requestIP() = %q, want rightmost untrusted X-Forwarded-For entry %q", got, "5.5.5.5")
	}
}

func TestRequestIPAllForwardedForEntriesTrusted(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "127.0.0.1", "10.0.0.0/8")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 10.0.0.1")

	if got := srv.requestIP(req); got != "10.0.0.2" {
		t.Fatalf("requestIP() = %q, want leftmost entry %q when all hops are trusted", got, "10.0.0.2")
	}
}

func TestRequestIPMalformedForwardedForFallsBackToPeer(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "127.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "not-an-ip, 5.5.5.5")

	if got := srv.requestIP(req); got != "5.5.5.5" {
		t.Fatalf("requestIP() = %q, want %q (rightmost valid entry before malformed data)", got, "5.5.5.5")
	}

	req.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := srv.requestIP(req); got != "127.0.0.1" {
		t.Fatalf("requestIP() = %q, want peer address fallback %q", got, "127.0.0.1")
	}
}

func TestRequestIPTrustedProxyCIDR(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "10.0.0.0/8")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:5678"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")

	if got := srv.requestIP(req); got != "203.0.113.1" {
		t.Fatalf("requestIP() = %q, want %q", got, "203.0.113.1")
	}
}

func TestRequestIPUntrustedProxyIgnoresHeader(t *testing.T) {
	srv := mustServerWithTrustedProxies(t, "127.0.0.1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("X-Forwarded-For", "9.9.9.9")

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
		ratelimit.Login:                     RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.Register:                  RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.ForgotPassword:            RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.ResetPassword:             RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.PublicResendVerification:  RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.AccountResendVerification: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.ChangePassword:            RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.ChangeEmail:               RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.RevokeSession:             RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.RevokeOtherSessions:       RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
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
		ratelimit.ForgotPassword: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
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
		ratelimit.ResetPassword: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
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
		ratelimit.AccountResendVerification: RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.ChangePassword:            RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.ChangeEmail:               RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.RevokeSession:             RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
		ratelimit.RevokeOtherSessions:       RateLimitPolicy{MaxRequests: 1, Window: time.Hour},
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

func TestRouteRateLimitLoginPerIPCeilingAcrossEmails(t *testing.T) {
	auth := &fakeAuthLookup{}
	srv := newAuthRouteTestServer(t, auth)
	// Per-email allowance is high so it is never the limiter here; the coarse
	// per-IP ceiling is what must cap total attempts from one IP spread across
	// many distinct emails (credential stuffing).
	srv.rateLimitPolicies = RateLimitPolicies{
		ratelimit.Login:      RateLimitPolicy{MaxRequests: 100, Window: time.Hour},
		ratelimit.LoginPerIP: RateLimitPolicy{MaxRequests: 2, Window: time.Hour},
	}
	routes := srv.Routes()

	for i, email := range []string{"a@example.com", "b@example.com"} {
		rec := postLoginFromIP(t, srv, routes, email, "10.0.0.1:1111")
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d (%s) = 429, want allowed under per-IP ceiling", i+1, email)
		}
	}

	blocked := postLoginFromIP(t, srv, routes, "c@example.com", "10.0.0.1:1111")
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("third distinct email from same IP = %d, want %d", blocked.Code, http.StatusTooManyRequests)
	}
	if blocked.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing on per-IP rejection")
	}

	// A different IP has its own bucket and is unaffected by the first IP's
	// exhausted ceiling.
	other := postLoginFromIP(t, srv, routes, "c@example.com", "10.0.0.2:2222")
	if other.Code == http.StatusTooManyRequests {
		t.Fatalf("different IP = 429, want allowed (separate per-IP bucket)")
	}
}

func postLoginFromIP(t *testing.T, srv *Server, routes http.Handler, email, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{
		"email":       []string{email},
		"password":    []string{"password"},
		csrfFieldName: []string{"csrf"},
	}
	req := httptest.NewRequest(http.MethodPost, paths.Login, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteAddr
	addCSRFCookieAndHeader(t, srv, req)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	return rec
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
