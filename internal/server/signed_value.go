package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// signValue returns a tamper-evident token of the form
// base64url(payload) + "." + base64url(HMAC-SHA256(key, payload)). It is the
// single signing primitive behind the flash, TOTP-pending, and passkey-ceremony
// cookies; each caller owns its own payload encoding and expiry handling.
func signValue(key, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyValue parses a token produced by signValue and returns the payload when
// the HMAC verifies. A missing separator, invalid base64, or signature mismatch
// returns ok=false.
func verifyValue(key []byte, token string) (payload []byte, ok bool) {
	encodedPayload, encodedSig, found := strings.Cut(token, ".")
	if !found {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(encodedSig)
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, false
	}
	return payload, true
}
