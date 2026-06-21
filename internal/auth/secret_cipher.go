package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var errInvalidCiphertext = errors.New("invalid ciphertext")

// secretCipher encrypts and decrypts secrets that must persist in a readable
// form but should not sit in the database as plaintext (currently the TOTP
// shared secret). It uses AES-256-GCM with a key derived from SECRET_KEY_BASE,
// so a leaked database backup does not expose users' second factors.
type secretCipher struct {
	aead cipher.AEAD
}

func newSecretCipher(key []byte) (*secretCipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &secretCipher{aead: aead}, nil
}

// encrypt returns base64(nonce || ciphertext) so the result is safe to store in
// a TEXT column.
func (c *secretCipher) encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (c *secretCipher) decrypt(encoded string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errInvalidCiphertext
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errInvalidCiphertext
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errInvalidCiphertext
	}
	return string(plaintext), nil
}
