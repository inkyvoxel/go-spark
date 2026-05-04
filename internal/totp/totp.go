package totp

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // TOTP (RFC 6238) mandates HMAC-SHA1
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

const (
	digits = 6
	period = 30
)

// BuildURI returns an otpauth:// URI suitable for QR code generation.
func BuildURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", digits))
	q.Set("period", fmt.Sprintf("%d", period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Generate returns the current 6-digit TOTP code for the given base32-encoded secret.
func Generate(secret string) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	return hotp(key, time.Now().Unix()/period), nil
}

// Verify checks a 6-digit TOTP code against a base32-encoded secret.
// It accepts codes from the previous, current, and next 30-second windows
// to tolerate clock skew.
func Verify(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != digits {
		return false
	}

	key, err := decodeSecret(secret)
	if err != nil {
		return false
	}

	counter := time.Now().Unix() / period
	for delta := int64(-1); delta <= 1; delta++ {
		if hotp(key, counter+delta) == code {
			return true
		}
	}
	return false
}

func decodeSecret(secret string) ([]byte, error) {
	secret = strings.TrimSpace(strings.ToUpper(secret))
	// Add padding if needed
	if r := len(secret) % 8; r != 0 {
		secret += strings.Repeat("=", 8-r)
	}
	return base32.StdEncoding.DecodeString(secret)
}

func hotp(key []byte, counter int64) string {
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(counter))

	mac := hmac.New(sha1.New, key)
	mac.Write(msg)
	h := mac.Sum(nil)

	offset := h[len(h)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(h[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", digits, truncated%uint32(math.Pow10(digits)))
}
