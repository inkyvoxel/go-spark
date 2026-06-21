package services

import (
	"context"
	"testing"
)

func TestAuthStoreEncryptsTOTPSecretAtRest(t *testing.T) {
	store := newTestauthStore(t)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	const plaintextSecret = "JBSWY3DPEHPK3PXP"
	if err := store.UpsertPendingTOTP(ctx, user.ID, plaintextSecret); err != nil {
		t.Fatalf("UpsertPendingTOTP() error = %v", err)
	}

	// The raw column must not contain the plaintext secret.
	var stored string
	if err := store.db.QueryRowContext(ctx, "SELECT secret FROM user_totp WHERE user_id = ?", user.ID).Scan(&stored); err != nil {
		t.Fatalf("select stored secret: %v", err)
	}
	if stored == plaintextSecret {
		t.Fatal("TOTP secret is stored as plaintext; expected ciphertext")
	}

	// Reading back through the store must return the original plaintext.
	record, err := store.GetTOTPByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetTOTPByUserID() error = %v", err)
	}
	if record.Secret != plaintextSecret {
		t.Fatalf("decrypted secret = %q, want %q", record.Secret, plaintextSecret)
	}
}

func TestAuthStoreReencryptsSecretOnEachWrite(t *testing.T) {
	store := newTestauthStore(t)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	const plaintextSecret = "JBSWY3DPEHPK3PXP"
	readStored := func() string {
		t.Helper()
		var stored string
		if err := store.db.QueryRowContext(ctx, "SELECT secret FROM user_totp WHERE user_id = ?", user.ID).Scan(&stored); err != nil {
			t.Fatalf("select stored secret: %v", err)
		}
		return stored
	}

	if err := store.UpsertPendingTOTP(ctx, user.ID, plaintextSecret); err != nil {
		t.Fatalf("UpsertPendingTOTP() error = %v", err)
	}
	first := readStored()

	if err := store.UpsertPendingTOTP(ctx, user.ID, plaintextSecret); err != nil {
		t.Fatalf("UpsertPendingTOTP() error = %v", err)
	}
	second := readStored()

	// A fresh random nonce per write means the ciphertext differs even for the
	// same plaintext, so a stored value never reveals secret reuse.
	if first == second {
		t.Fatal("ciphertext did not change between writes; nonce may be reused")
	}
}
