// Package totp implements the slice of RFC 6238 (TOTP) this app needs, built on
// the maintained github.com/pquerna/otp library so the HMAC/HOTP crypto is not
// hand-rolled. It adds the two app-specific pieces the library does not provide:
// the otpauth:// URI used for QR codes, and counter-returning verification so
// the caller can record the matched time step for replay protection.
package totp

import (
	"crypto/subtle"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
)

const (
	digits = 6
	period = 30
)

// BuildURI returns an otpauth:// URI suitable for QR code generation. The
// parameters match the library defaults that Generate and VerifyWithCounter
// rely on (SHA1, 6 digits, 30-second period).
func BuildURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", strconv.Itoa(digits))
	q.Set("period", strconv.Itoa(period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Generate returns the current 6-digit TOTP code for the given base32-encoded secret.
func Generate(secret string) (string, error) {
	return totp.GenerateCode(secret, time.Now())
}

// VerifyWithCounter checks a 6-digit TOTP code against a base32-encoded secret,
// accepting the previous, current, and next 30-second windows to tolerate clock
// skew. It returns the time-step counter the code matched. Callers should
// persist the counter and reject codes whose counter is not strictly greater
// than the last accepted one, so a captured code cannot be replayed within its
// validity window (RFC 6238 section 5.2).
func VerifyWithCounter(secret, code string) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != digits {
		return 0, false
	}

	counter := time.Now().Unix() / period
	matched := int64(0)
	ok := false
	for delta := int64(-1); delta <= 1; delta++ {
		step := counter + delta
		// pquerna performs the HMAC-SHA1 HOTP computation; we only locate which
		// accepted window matched so the caller can record its counter.
		candidate, err := totp.GenerateCode(secret, time.Unix(step*period, 0))
		if err != nil {
			return 0, false
		}
		// Compare every window without early exit so timing reveals nothing about
		// which (if any) window matched.
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 && !ok {
			matched = step
			ok = true
		}
	}
	return matched, ok
}
