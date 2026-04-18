package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"time"
)

const (
	csrfCookieName = "csrf_token"
	csrfFieldName  = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"
)

func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := csrfTokenFromCookie(r)
		if token == "" {
			var err error
			token, err = newCSRFToken()
			if err != nil {
				s.logger.Error("generate csrf token", "err", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			s.setCSRFCookie(w, r, token)
		}

		if isUnsafeMethod(r.Method) {
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
		}

		next.ServeHTTP(w, r.WithContext(contextWithCSRFToken(r.Context(), token)))
	})
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
		Expires:  time.Now().UTC().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func newCSRFToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}
