package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	// Separate, path-scoped cookies keep the registration and login ceremony
	// state isolated to the endpoints that complete them.
	passkeyRegisterCookieName = "passkey_register"
	passkeyRegisterCookiePath = "/account/passkeys"
	passkeyLoginCookieName    = "passkey_login"
	passkeyLoginCookiePath    = "/login/passkey"
)

// passkeySessionKey derives the HMAC key used to sign the WebAuthn ceremony
// cookie from the root cookie-signing key, matching the totp_pending cookie pattern.
func (s *Server) passkeySessionKey() []byte {
	return services.DeriveKey(s.cookieSigningKey, "passkey_session")
}

// setPasskeySessionCookie stores the WebAuthn SessionData in a short-lived,
// HMAC-signed cookie. The challenge it carries is already public (it is sent to
// the client), so signing for integrity is sufficient; it must not be
// client-modifiable, which the signature guarantees.
func (s *Server) setPasskeySessionCookie(w http.ResponseWriter, r *http.Request, name, path string, session *webauthn.SessionData) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}

	mac := hmac.New(sha256.New, s.passkeySessionKey())
	mac.Write(payload)
	value := base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  session.Expires,
		MaxAge:   passkeyCookieMaxAge(session.Expires),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func passkeyCookieMaxAge(expiresAt time.Time) int {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		return 1
	}
	return maxAge
}

// passkeySessionFromCookie verifies the cookie signature and expiry and returns
// the embedded SessionData. Returns false if missing, tampered, or expired.
func (s *Server) passkeySessionFromCookie(r *http.Request, name string) (webauthn.SessionData, bool) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return webauthn.SessionData{}, false
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return webauthn.SessionData{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return webauthn.SessionData{}, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return webauthn.SessionData{}, false
	}

	mac := hmac.New(sha256.New, s.passkeySessionKey())
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return webauthn.SessionData{}, false
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(payload, &session); err != nil {
		return webauthn.SessionData{}, false
	}
	if !session.Expires.IsZero() && time.Now().UTC().After(session.Expires) {
		return webauthn.SessionData{}, false
	}
	return session, true
}

func (s *Server) clearPasskeySessionCookie(w http.ResponseWriter, r *http.Request, name, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
}
