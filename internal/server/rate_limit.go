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

	"github.com/inkyvoxel/go-spark/internal/ratelimit"
)

const (
	cleanupEveryNRateLimitCalls = 256
	maxRateLimitEntries         = 50_000
)

// RateLimitPolicy and RateLimitPolicies are the shared ratelimit types; the
// vocabulary (action names, defaults) lives in the ratelimit package so the
// config loader and this server agree without copying parallel structs.
type RateLimitPolicy = ratelimit.Policy

type RateLimitPolicies = ratelimit.Policies

type rateLimitKeyFunc func(*http.Request) (key string, keyType string)

// fixedWindowRateLimiter is a simple in-memory fixed-window counter, which is
// correct for single-server deployments. To scale the web process across
// multiple servers, swap s.rateLimiter for a shared-store implementation (e.g.
// Redis) exposing the same Allow method. Note: multi-server rate limiting also
// requires replacing SQLite with a network-accessible database, as both share
// the same single-server boundary.
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

func (s *Server) ensureRateLimiting() {
	if s.rateLimiter == nil {
		s.rateLimiter = newFixedWindowRateLimiter()
	}
	s.rateLimitPolicies = ratelimit.Resolve(s.rateLimitPolicies)
}

// rateLimitSpec pairs a named policy with the key function that buckets
// requests for it. A single route may be guarded by several specs (see
// withRateLimits).
type rateLimitSpec struct {
	name   string
	policy RateLimitPolicy
	keyFn  rateLimitKeyFunc
}

func rateLimit(name string, policy RateLimitPolicy, keyFn rateLimitKeyFunc) rateLimitSpec {
	return rateLimitSpec{name: name, policy: policy, keyFn: keyFn}
}

// withRateLimits guards next with one or more rate limiters, evaluated in
// order. The first spec that rejects the request short-circuits with 429, so
// place coarse per-IP ceilings before fine-grained per-email/per-user limiters:
// an IP over its ceiling is rejected without consuming the narrower bucket.
func (s *Server) withRateLimits(next http.Handler, specs ...rateLimitSpec) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		for _, spec := range specs {
			key, keyType := spec.keyFn(r)
			allowed, retryAfter := s.rateLimiter.Allow(spec.name+"|"+key, spec.policy, now)
			if !allowed {
				s.writeRateLimited(w, r, spec.name, keyType, key, retryAfter)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeRateLimited(w http.ResponseWriter, r *http.Request, policyName, keyType, key string, retryAfter time.Duration) {
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

func (s *Server) rateLimitKeyByIP() rateLimitKeyFunc {
	return func(r *http.Request) (string, string) {
		return "ip:" + s.requestIP(r), "ip"
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
		if clientIP := s.clientIPFromForwardedFor(r.Header.Get("X-Forwarded-For")); clientIP != "" {
			return clientIP
		}
	}

	return host
}

// clientIPFromForwardedFor walks X-Forwarded-For from right to left and
// returns the first hop that is not a trusted proxy. Proxies append the
// connecting address to this header, so rightmost entries were added by our
// own proxies while leftmost entries are client-supplied and spoofable.
// Returns "" if the header is empty or contains an unparsable entry, in which
// case the caller falls back to the direct peer address.
func (s *Server) clientIPFromForwardedFor(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	entries := strings.Split(header, ",")
	candidate := ""
	for i := len(entries) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(entries[i]))
		if ip == nil {
			return ""
		}
		candidate = ip.String()
		if !s.isTrustedProxyIP(ip) {
			return candidate
		}
	}

	// Every hop was a trusted proxy; the leftmost entry is the originator.
	return candidate
}

func (s *Server) isTrustedProxy(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return s.isTrustedProxyIP(ip)
}

func (s *Server) isTrustedProxyIP(ip net.IP) bool {
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
