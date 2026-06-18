package server

import (
	"strings"
	"testing"
)

func TestSignVerifyValue(t *testing.T) {
	key := []byte("test-signing-key")
	payload := []byte(`{"hello":"world"}`)
	token := signValue(key, payload)

	got, ok := verifyValue(key, token)
	if !ok || string(got) != string(payload) {
		t.Fatalf("round-trip = (%q, %v), want (%q, true)", got, ok, payload)
	}

	if _, ok := verifyValue([]byte("different-key"), token); ok {
		t.Fatal("verifyValue accepted a token signed with a different key")
	}

	tampered := token[:strings.LastIndex(token, ".")+1] + "AAAA"
	if _, ok := verifyValue(key, tampered); ok {
		t.Fatal("verifyValue accepted a tampered signature")
	}

	if _, ok := verifyValue(key, "no-separator"); ok {
		t.Fatal("verifyValue accepted a token without a separator")
	}
}
