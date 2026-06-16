package database

import (
	"errors"
	"testing"
)

func TestSecretCipherRoundTrip(t *testing.T) {
	cipher, err := newSecretCipher(testTOTPSecretKey)
	if err != nil {
		t.Fatalf("newSecretCipher() error = %v", err)
	}

	for _, plaintext := range []string{"", "JBSWY3DPEHPK3PXP", "a longer secret value with spaces"} {
		encoded, err := cipher.encrypt(plaintext)
		if err != nil {
			t.Fatalf("encrypt(%q) error = %v", plaintext, err)
		}
		if encoded == plaintext && plaintext != "" {
			t.Fatalf("encrypt(%q) returned plaintext", plaintext)
		}

		decoded, err := cipher.decrypt(encoded)
		if err != nil {
			t.Fatalf("decrypt() error = %v", err)
		}
		if decoded != plaintext {
			t.Fatalf("decrypt() = %q, want %q", decoded, plaintext)
		}
	}
}

func TestSecretCipherRejectsTamperedCiphertext(t *testing.T) {
	cipher, err := newSecretCipher(testTOTPSecretKey)
	if err != nil {
		t.Fatalf("newSecretCipher() error = %v", err)
	}

	encoded, err := cipher.encrypt("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	tampered := []byte(encoded)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := cipher.decrypt(string(tampered)); !errors.Is(err, errInvalidCiphertext) {
		t.Fatalf("decrypt(tampered) error = %v, want errInvalidCiphertext", err)
	}

	if _, err := cipher.decrypt("not base64!!"); !errors.Is(err, errInvalidCiphertext) {
		t.Fatalf("decrypt(garbage) error = %v, want errInvalidCiphertext", err)
	}
}

func TestSecretCipherWrongKeyFailsToDecrypt(t *testing.T) {
	cipher, err := newSecretCipher(testTOTPSecretKey)
	if err != nil {
		t.Fatalf("newSecretCipher() error = %v", err)
	}
	encoded, err := cipher.encrypt("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	other, err := newSecretCipher([]byte("fedcba9876543210fedcba9876543210"))
	if err != nil {
		t.Fatalf("newSecretCipher() error = %v", err)
	}
	if _, err := other.decrypt(encoded); !errors.Is(err, errInvalidCiphertext) {
		t.Fatalf("decrypt(wrong key) error = %v, want errInvalidCiphertext", err)
	}
}

func TestNewSecretCipherRejectsBadKeyLength(t *testing.T) {
	if _, err := newSecretCipher([]byte("too-short")); err == nil {
		t.Fatal("newSecretCipher() with short key error = nil, want error")
	}
}
