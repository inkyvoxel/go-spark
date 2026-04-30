package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	csrfCookieName = "csrf_token"
	csrfFieldName  = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"

	csrfTokenVersion          = "v1"
	csrfAnonymousSessionHash  = "anon"
	csrfCookieTTL             = 24 * time.Hour
	defaultTestCSRFSigningKey = "insecure-default-csrf-signing-key"
)

type csrfTokenPayload struct {
	SessionHash string `json:"sid"`
	ExpiresAt   int64  `json:"exp"`
	Nonce       string `json:"n"`
}

func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionToken := sessionTokenFromRequest(r)
		sessionHash := csrfSessionHash(sessionToken)
		token := csrfTokenFromCookie(r)

		if token == "" {
			var err error
			token, err = s.newSignedCSRFToken(sessionHash, time.Now().UTC())
			if err != nil {
				s.loggerForRequest(r).Error("generate csrf token", "err", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			s.setCSRFCookie(w, r, token)
		}

		if !isUnsafeMethod(r.Method) {
			if !s.validSignedCSRFToken(token, sessionHash, time.Now().UTC()) {
				rotated, err := s.newSignedCSRFToken(sessionHash, time.Now().UTC())
				if err != nil {
					s.loggerForRequest(r).Error("rotate csrf token", "err", err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				token = rotated
				s.setCSRFCookie(w, r, token)
			}
			next.ServeHTTP(w, r.WithContext(contextWithCSRFToken(r.Context(), token)))
			return
		}

		requestToken, err := csrfTokenFromRequest(r)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		if !validCSRFToken(token, requestToken) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		if !s.validSignedCSRFToken(token, sessionHash, time.Now().UTC()) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		if !s.validRequestSourceOrigin(r) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r.WithContext(contextWithCSRFToken(r.Context(), token)))
	})
}

func (s *Server) validRequestSourceOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if strings.EqualFold(origin, "null") {
		origin = ""
	}
	if origin != "" {
		normalized, ok := normalizeHeaderOrigin(origin)
		if !ok || s.appBaseOrigin == "" {
			return false
		}
		return normalized == s.appBaseOrigin
	}

	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer == "" {
		return true
	}
	normalized, ok := normalizeRefererOrigin(referer)
	if !ok || s.appBaseOrigin == "" {
		return false
	}
	return normalized == s.appBaseOrigin
}

func normalizeHeaderOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

func normalizeRefererOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

func sessionTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func csrfSessionHash(sessionToken string) string {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return csrfAnonymousSessionHash
	}
	return tokenHash(sessionToken)
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) newSignedCSRFToken(sessionHash string, now time.Time) (string, error) {
	nonce, err := randomTokenBytes(32)
	if err != nil {
		return "", err
	}

	payload := csrfTokenPayload{
		SessionHash: sessionHash,
		ExpiresAt:   now.Add(csrfCookieTTL).Unix(),
		Nonce:       base64.RawURLEncoding.EncodeToString(nonce),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signedInput := csrfTokenVersion + "." + payloadEncoded
	signature := signCSRFToken(s.csrfSigningSecret(), signedInput)
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)
	return signedInput + "." + signatureEncoded, nil
}

func (s *Server) validSignedCSRFToken(token, sessionHash string, now time.Time) bool {
	signedInput, payload, signature, ok := parseSignedCSRFToken(token)
	if !ok {
		return false
	}

	expectedSignature := signCSRFToken(s.csrfSigningSecret(), signedInput)
	if len(expectedSignature) != len(signature) || subtle.ConstantTimeCompare(expectedSignature, signature) != 1 {
		return false
	}
	if payload.ExpiresAt <= now.Unix() {
		return false
	}
	if payload.SessionHash == "" || payload.SessionHash != sessionHash {
		return false
	}
	return true
}

func parseSignedCSRFToken(token string) (string, csrfTokenPayload, []byte, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", csrfTokenPayload{}, nil, false
	}
	if parts[0] != csrfTokenVersion {
		return "", csrfTokenPayload{}, nil, false
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", csrfTokenPayload{}, nil, false
	}

	var payload csrfTokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", csrfTokenPayload{}, nil, false
	}
	if payload.Nonce == "" {
		return "", csrfTokenPayload{}, nil, false
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", csrfTokenPayload{}, nil, false
	}

	return parts[0] + "." + parts[1], payload, signature, true
}

func signCSRFToken(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func randomTokenBytes(size int) ([]byte, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

func csrfTokenFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func csrfTokenFromRequest(r *http.Request) (string, error) {
	if token := r.Header.Get(csrfHeaderName); token != "" {
		return token, nil
	}
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	return r.FormValue(csrfFieldName), nil
}

func validCSRFToken(cookieToken, requestToken string) bool {
	if cookieToken == "" || requestToken == "" {
		return false
	}
	if len(cookieToken) != len(requestToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(requestToken)) == 1
}

func (s *Server) setCSRFCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().UTC().Add(csrfCookieTTL),
		MaxAge:   int(csrfCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCSRFCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) rotateCSRFCookieForSession(w http.ResponseWriter, r *http.Request, sessionToken string) error {
	token, err := s.newSignedCSRFToken(csrfSessionHash(sessionToken), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("generate csrf token: %w", err)
	}
	s.setCSRFCookie(w, r, token)
	return nil
}

func (s *Server) csrfSigningSecret() []byte {
	if len(s.csrfSigningKey) != 0 {
		return s.csrfSigningKey
	}
	return []byte(defaultTestCSRFSigningKey)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}
