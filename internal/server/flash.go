package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const flashCookieName = "flash"

type flashMessage struct {
	Type    string `json:"t"`
	Message string `json:"m"`
}

func flashSuccess(message string) flashMessage {
	return flashMessage{Type: "success", Message: message}
}

func flashError(message string) flashMessage {
	return flashMessage{Type: "error", Message: message}
}

func (s *Server) setFlash(w http.ResponseWriter, r *http.Request, msg flashMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h := hmac.New(sha256.New, s.flashSigningKey())
	h.Write(payload)
	sig := h.Sum(nil)

	value := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig)

	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int((5 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) popFlash(w http.ResponseWriter, r *http.Request) (flashMessage, bool) {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil {
		return flashMessage{}, false
	}

	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return flashMessage{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return flashMessage{}, false
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return flashMessage{}, false
	}

	h := hmac.New(sha256.New, s.flashSigningKey())
	h.Write(payload)
	if !hmac.Equal(sig, h.Sum(nil)) {
		return flashMessage{}, false
	}

	var msg flashMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return flashMessage{}, false
	}

	return msg, true
}

func (s *Server) flashSigningKey() []byte {
	if len(s.flashKey) != 0 {
		return s.flashKey
	}
	return deriveKey([]byte(defaultTestCSRFSigningKey), "flash")
}

// deriveKey produces a purpose-scoped key from a root secret using HMAC-SHA256.
// All signing keys in this application are derived from SecretKeyBase via this
// function, matching the single-root-secret pattern (cf. Rails secret_key_base).
func deriveKey(base []byte, purpose string) []byte {
	h := hmac.New(sha256.New, base)
	h.Write([]byte(purpose))
	return h.Sum(nil)
}
