package server

import (
	"fmt"
	"net/http"
)

// defaultCrossOriginProtection rejects cross-origin unsafe requests using the
// Sec-Fetch-Site header (falling back to Origin vs Host) and trusts no extra
// origins. Servers built via New get one configured with the app's own origin.
var defaultCrossOriginProtection = http.NewCrossOriginProtection()

// newCrossOriginProtection builds the origin-based CSRF defense. The app's own
// origin is registered as trusted so requests still pass behind a proxy that
// rewrites the Host (e.g. TLS termination) for the rare browsers that omit
// Sec-Fetch-Site.
func newCrossOriginProtection(appBaseOrigin string) (*http.CrossOriginProtection, error) {
	protection := http.NewCrossOriginProtection()
	if appBaseOrigin == "" {
		return protection, nil
	}
	if err := protection.AddTrustedOrigin(appBaseOrigin); err != nil {
		return nil, fmt.Errorf("trust app origin %q: %w", appBaseOrigin, err)
	}
	return protection, nil
}

func (s *Server) crossOriginProtection() *http.CrossOriginProtection {
	if s.crossOrigin != nil {
		return s.crossOrigin
	}
	return defaultCrossOriginProtection
}

// csrf blocks cross-origin state-changing requests. It relies on the standard
// library's http.CrossOriginProtection — Sec-Fetch-Site with an Origin-vs-Host
// fallback — paired with the SameSite=Lax session cookie. That combination is
// the modern CSRF defense and needs no per-form token. Safe methods are never
// blocked.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) {
			if err := s.crossOriginProtection().Check(r); err != nil {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}
