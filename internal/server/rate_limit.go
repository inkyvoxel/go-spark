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

const cleanupEveryNRateLimitCalls = 256

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
}

type rateLimitKeyFunc func(*http.Request) (key string, keyType string)

type rateLimitStore interface {
	Allow(bucketKey string, policy RateLimitPolicy, now time.Time) (bool, time.Duration)
}

type inMemoryRateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
	calls   uint64
}

type rateLimitEntry struct {
	Count   int
	ResetAt time.Time
}

func newInMemoryRateLimiter() *inMemoryRateLimiter {
	return &inMemoryRateLimiter{
		entries: make(map[string]rateLimitEntry),
	}
}

func (l *inMemoryRateLimiter) Allow(bucketKey string, policy RateLimitPolicy, now time.Time) (bool, time.Duration) {
	if policy.MaxRequests <= 0 || policy.Window <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.calls++
	if l.calls%cleanupEveryNRateLimitCalls == 0 {
		l.cleanupExpired(now)
	}

	entry, ok := l.entries[bucketKey]
	if !ok || !now.Before(entry.ResetAt) {
		l.entries[bucketKey] = rateLimitEntry{
			Count:   1,
			ResetAt: now.Add(policy.Window),
		}
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

func (l *inMemoryRateLimiter) cleanupExpired(now time.Time) {
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
		s.rateLimiter = newInMemoryRateLimiter()
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

func rateLimitKeyByIPAndEmail(formField string) rateLimitKeyFunc {
	return func(r *http.Request) (string, string) {
		ip := requestIP(r)
		_ = r.ParseForm()
		email, ok := normalizedEmailForRateLimit(r.FormValue(formField))
		if !ok {
			return "ip:" + ip, "ip"
		}
		return fmt.Sprintf("ip:%s|email:%s", ip, email), "ip_email"
	}
}

func rateLimitKeyByIPAndResetTokenCookie() rateLimitKeyFunc {
	return func(r *http.Request) (string, string) {
		ip := requestIP(r)
		token := resetTokenFromCookie(r)
		if token == "" {
			return "ip:" + ip, "ip"
		}
		return fmt.Sprintf("ip:%s|reset_token_hash:%s", ip, hashRateLimitKey(token)), "ip_reset_token"
	}
}

func rateLimitKeyByIPAndUser() rateLimitKeyFunc {
	return func(r *http.Request) (string, string) {
		ip := requestIP(r)
		user, ok := currentUser(r.Context())
		if !ok {
			return "ip:" + ip, "ip"
		}
		return fmt.Sprintf("ip:%s|user:%d", ip, user.ID), "ip_user"
	}
}

func requestIP(r *http.Request) string {
	remoteAddr := strings.TrimSpace(r.RemoteAddr)
	if remoteAddr == "" {
		return "unknown"
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	if host == "" {
		return remoteAddr
	}
	return host
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
