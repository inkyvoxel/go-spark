package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cleanupEveryNRateLimitCalls = 256
	maxRateLimitEntries         = 50_000
)

type RateLimitPolicy struct {
	MaxRequests int
	Window      time.Duration
}

type RateLimitPolicies struct {
	Login                     RateLimitPolicy
	Register                  RateLimitPolicy
	ForgotPassword            RateLimitPolicy
	ResetPassword             RateLimitPolicy
	PublicResendVerification  RateLimitPolicy
	AccountResendVerification RateLimitPolicy
	ChangePassword            RateLimitPolicy
	ChangeEmail               RateLimitPolicy
	RevokeSession             RateLimitPolicy
	RevokeOtherSessions       RateLimitPolicy
	DeleteAccount             RateLimitPolicy
}

var defaultRateLimitPolicies = RateLimitPolicies{
	Login:                     RateLimitPolicy{MaxRequests: 5, Window: time.Minute},
	Register:                  RateLimitPolicy{MaxRequests: 3, Window: 10 * time.Minute},
	ForgotPassword:            RateLimitPolicy{MaxRequests: 3, Window: 15 * time.Minute},
	ResetPassword:             RateLimitPolicy{MaxRequests: 5, Window: 15 * time.Minute},
	PublicResendVerification:  RateLimitPolicy{MaxRequests: 3, Window: 15 * time.Minute},
	AccountResendVerification: RateLimitPolicy{MaxRequests: 5, Window: 15 * time.Minute},
	ChangePassword:            RateLimitPolicy{MaxRequests: 5, Window: 15 * time.Minute},
	ChangeEmail:               RateLimitPolicy{MaxRequests: 5, Window: 15 * time.Minute},
	RevokeSession:             RateLimitPolicy{MaxRequests: 20, Window: 15 * time.Minute},
	RevokeOtherSessions:       RateLimitPolicy{MaxRequests: 10, Window: 15 * time.Minute},
	DeleteAccount:             RateLimitPolicy{MaxRequests: 3, Window: 15 * time.Minute},
}

type rateLimitKeyFunc func(*http.Request) (key string, keyType string)

type rateLimitStore interface {
	Allow(bucketKey string, policy RateLimitPolicy, now time.Time) (bool, time.Duration)
}

// fixedWindowRateLimiter is a simple in-memory fixed-window counter.
// Each bucket tracks a count and a reset time. When the window expires, the
// count resets. Note the boundary burst: a caller can make MaxRequests calls
// just before reset and MaxRequests calls just after, for a short burst of
// 2×MaxRequests. The conservative default policies keep this acceptable.
type fixedWindowRateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
	calls   uint64
}

type rateLimitEntry struct {
	Count   int
	ResetAt time.Time
}

func newFixedWindowRateLimiter() *fixedWindowRateLimiter {
	return &fixedWindowRateLimiter{
		entries: make(map[string]rateLimitEntry),
	}
}

func (l *fixedWindowRateLimiter) Allow(bucketKey string, policy RateLimitPolicy, now time.Time) (bool, time.Duration) {
	if policy.MaxRequests <= 0 || policy.Window <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.calls++
	if l.calls%cleanupEveryNRateLimitCalls == 0 {
		l.cleanupExpired(now)
		l.calls = 0
	}

	entry, ok := l.entries[bucketKey]
	if !ok {
		if len(l.entries) >= maxRateLimitEntries {
			return false, policy.Window
		}
		l.entries[bucketKey] = rateLimitEntry{Count: 1, ResetAt: now.Add(policy.Window)}
		return true, 0
	}
	if !now.Before(entry.ResetAt) {
		l.entries[bucketKey] = rateLimitEntry{Count: 1, ResetAt: now.Add(policy.Window)}
		return true, 0
	}

	if entry.Count < policy.MaxRequests {
		entry.Count++
		l.entries[bucketKey] = entry
		return true, 0
	}

	retryAfter := entry.ResetAt.Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return false, retryAfter
}

func (l *fixedWindowRateLimiter) cleanupExpired(now time.Time) {
	for key, entry := range l.entries {
		if !now.Before(entry.ResetAt) {
			delete(l.entries, key)
		}
	}
}

func rateLimitPoliciesWithDefaults(policies RateLimitPolicies) RateLimitPolicies {
	return RateLimitPolicies{
		Login:                     mergeRateLimitPolicy(defaultRateLimitPolicies.Login, policies.Login),
		Register:                  mergeRateLimitPolicy(defaultRateLimitPolicies.Register, policies.Register),
		ForgotPassword:            mergeRateLimitPolicy(defaultRateLimitPolicies.ForgotPassword, policies.ForgotPassword),
		ResetPassword:             mergeRateLimitPolicy(defaultRateLimitPolicies.ResetPassword, policies.ResetPassword),
		PublicResendVerification:  mergeRateLimitPolicy(defaultRateLimitPolicies.PublicResendVerification, policies.PublicResendVerification),
		AccountResendVerification: mergeRateLimitPolicy(defaultRateLimitPolicies.AccountResendVerification, policies.AccountResendVerification),
		ChangePassword:            mergeRateLimitPolicy(defaultRateLimitPolicies.ChangePassword, policies.ChangePassword),
		ChangeEmail:               mergeRateLimitPolicy(defaultRateLimitPolicies.ChangeEmail, policies.ChangeEmail),
		RevokeSession:             mergeRateLimitPolicy(defaultRateLimitPolicies.RevokeSession, policies.RevokeSession),
		RevokeOtherSessions:       mergeRateLimitPolicy(defaultRateLimitPolicies.RevokeOtherSessions, policies.RevokeOtherSessions),
		DeleteAccount:             mergeRateLimitPolicy(defaultRateLimitPolicies.DeleteAccount, policies.DeleteAccount),
	}
}

func mergeRateLimitPolicy(defaultPolicy, override RateLimitPolicy) RateLimitPolicy {
	policy := defaultPolicy
	if override.MaxRequests > 0 {
		policy.MaxRequests = override.MaxRequests
	}
	if override.Window > 0 {
		policy.Window = override.Window
	}
	return policy
}

func (s *Server) ensureRateLimiting() {
	if s.rateLimiter == nil {
		s.rateLimiter = newFixedWindowRateLimiter()
	}
	s.rateLimitPolicies = rateLimitPoliciesWithDefaults(s.rateLimitPolicies)
}

func (s *Server) withRateLimit(policyName string, policy RateLimitPolicy, keyFn rateLimitKeyFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, keyType := keyFn(r)
		allowed, retryAfter := s.rateLimiter.Allow(policyName+"|"+key, policy, time.Now().UTC())
		if allowed {
			next.ServeHTTP(w, r)
			return
		}

		retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
		if retryAfterSeconds < 1 {
			retryAfterSeconds = 1
		}

		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
		s.loggerForRequest(r).Warn(
			"rate limit exceeded",
			"policy", policyName,
			"path", r.URL.Path,
			"key_type", keyType,
			"key_hash", hashRateLimitKey(key),
			"retry_after_seconds", retryAfterSeconds,
		)
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
	})
}

func (s *Server) rateLimitKeyByIPAndEmail(formField string) rateLimitKeyFunc {
	return func(r *http.Request) (string, string) {
		ip := s.requestIP(r)
		_ = r.ParseForm()
		email, ok := normalizedEmailForRateLimit(r.FormValue(formField))
		if !ok {
			return "ip:" + ip, "ip"
		}
		return fmt.Sprintf("ip:%s|email:%s", ip, email), "ip_email"
	}
}

func (s *Server) rateLimitKeyByIPAndResetTokenCookie() rateLimitKeyFunc {
	return func(r *http.Request) (string, string) {
		ip := s.requestIP(r)
		token := resetTokenFromCookie(r)
		if token == "" {
			return "ip:" + ip, "ip"
		}
		return fmt.Sprintf("ip:%s|reset_token_hash:%s", ip, hashRateLimitKey(token)), "ip_reset_token"
	}
}

func (s *Server) rateLimitKeyByIPAndUser() rateLimitKeyFunc {
	return func(r *http.Request) (string, string) {
		ip := s.requestIP(r)
		user, ok := currentUser(r.Context())
		if !ok {
			return "ip:" + ip, "ip"
		}
		return fmt.Sprintf("ip:%s|user:%d", ip, user.ID), "ip_user"
	}
}

func (s *Server) requestIP(r *http.Request) string {
	remoteAddr := strings.TrimSpace(r.RemoteAddr)
	if remoteAddr == "" {
		return "unknown"
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if host == "" {
		host = remoteAddr
	}

	if len(s.trustedProxies) > 0 && s.isTrustedProxy(host) {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed.String()
			}
		}
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
				if parsed := net.ParseIP(ip); parsed != nil {
					return parsed.String()
				}
			}
		}
	}

	return host
}

func (s *Server) isTrustedProxy(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range s.trustedProxies {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func hashRateLimitKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:6])
}

func normalizedEmailForRateLimit(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}

	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr.Address == "" {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(addr.Address)), true
}
