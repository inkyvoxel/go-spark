package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/inkyvoxel/go-spark/internal/secret"
)

const flashCookieName = "flash"

// defaultTestFlashSigningKey is the fallback signing key used only when a Server
// is constructed without a derived flashKey (e.g. in tests). Real servers built
// via New always derive flashKey from SECRET_KEY_BASE.
const defaultTestFlashSigningKey = "insecure-default-flash-signing-key"

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

	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    signValue(s.flashSigningKey(), payload),
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

	payload, ok := verifyValue(s.flashSigningKey(), cookie.Value)
	if !ok {
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
	return secret.DeriveKey([]byte(defaultTestFlashSigningKey), "flash")
}
