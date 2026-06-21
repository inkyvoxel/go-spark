// Package secret derives purpose-scoped keys from a single root secret.
package secret

import (
	"crypto/hmac"
	"crypto/sha256"
)

// DeriveKey produces a purpose-scoped key from a root secret using
// HMAC-SHA256. All signing and encryption subkeys in this application are
// derived from a single root secret via this function, matching the
// single-root-secret pattern (cf. Rails secret_key_base).
func DeriveKey(base []byte, purpose string) []byte {
	h := hmac.New(sha256.New, base)
	h.Write([]byte(purpose))
	return h.Sum(nil)
}
