package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/inkyvoxel/go-spark/internal/email"
)

func newPasskeyTestService(t *testing.T) (*AuthService, *fakeAuthStore) {
	t.Helper()

	store := newFakeAuthStore()
	service := mustNewAuthService(t, store, AuthOptions{
		SessionDuration:       time.Hour,
		PasswordMinLen:        8,
		Argon2idMemoryKiB:     64,
		Argon2idIterations:    1,
		Argon2idParallelism:   1,
		WebAuthnRPID:          "localhost",
		WebAuthnRPDisplayName: "Test App",
		WebAuthnRPOrigins:     []string{"http://localhost:8080"},
		ConfirmationEmail: email.AccountConfirmationOptions{
			AppBaseURL: "http://localhost:8080",
			From:       "Go Spark <hello@example.com>",
		},
	})
	return service, store
}

func TestPasskeysEnabledReflectsConfiguration(t *testing.T) {
	enabled, _ := newPasskeyTestService(t)
	if !enabled.PasskeysEnabled() {
		t.Fatal("PasskeysEnabled() = false, want true when RP is configured")
	}

	disabled := newTestAuthService(t)
	if disabled.PasskeysEnabled() {
		t.Fatal("PasskeysEnabled() = true, want false when RP is not configured")
	}
}

func TestBeginPasskeyRegistrationReturnsErrorWhenDisabled(t *testing.T) {
	service := newTestAuthService(t)
	store := service.store.(*fakeAuthStore)
	user, _ := store.CreateUser(context.Background(), "user@example.com", "hash")

	if _, _, err := service.BeginPasskeyRegistration(context.Background(), user.ID); !errors.Is(err, ErrPasskeysDisabled) {
		t.Fatalf("BeginPasskeyRegistration() error = %v, want %v", err, ErrPasskeysDisabled)
	}
}

func TestBeginPasskeyRegistrationOptions(t *testing.T) {
	service, store := newPasskeyTestService(t)
	ctx := context.Background()
	user, _ := store.CreateUser(ctx, "user@example.com", "hash")

	// An already-registered credential must be excluded from re-registration.
	if err := store.CreateWebAuthnCredential(ctx, CreateWebAuthnCredentialParams{
		UserID:       user.ID,
		CredentialID: []byte("existing-credential"),
		PublicKey:    []byte("pk"),
		Transports:   []string{},
	}); err != nil {
		t.Fatalf("CreateWebAuthnCredential() error = %v", err)
	}

	creation, session, err := service.BeginPasskeyRegistration(ctx, user.ID)
	if err != nil {
		t.Fatalf("BeginPasskeyRegistration() error = %v", err)
	}

	if creation.Response.RelyingParty.ID != "localhost" {
		t.Fatalf("RP ID = %q, want localhost", creation.Response.RelyingParty.ID)
	}
	if creation.Response.AuthenticatorSelection.UserVerification != protocol.VerificationRequired {
		t.Fatalf("UserVerification = %q, want required", creation.Response.AuthenticatorSelection.UserVerification)
	}
	if creation.Response.AuthenticatorSelection.ResidentKey != protocol.ResidentKeyRequirementRequired {
		t.Fatalf("ResidentKey = %q, want required", creation.Response.AuthenticatorSelection.ResidentKey)
	}
	if len(creation.Response.CredentialExcludeList) != 1 {
		t.Fatalf("exclude list length = %d, want 1", len(creation.Response.CredentialExcludeList))
	}
	if session == nil || session.Challenge == "" {
		t.Fatal("session data should carry a challenge")
	}
}

func TestBeginPasskeyLoginIsDiscoverable(t *testing.T) {
	service, _ := newPasskeyTestService(t)

	assertion, session, err := service.BeginPasskeyLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginPasskeyLogin() error = %v", err)
	}
	if len(assertion.Response.AllowedCredentials) != 0 {
		t.Fatalf("allowed credentials = %d, want 0 (discoverable)", len(assertion.Response.AllowedCredentials))
	}
	if assertion.Response.UserVerification != protocol.VerificationRequired {
		t.Fatalf("UserVerification = %q, want required", assertion.Response.UserVerification)
	}
	if session == nil || session.Challenge == "" {
		t.Fatal("session data should carry a challenge")
	}
}

func TestPasskeyManagementOverStore(t *testing.T) {
	service, store := newPasskeyTestService(t)
	ctx := context.Background()
	user, _ := store.CreateUser(ctx, "user@example.com", "hash")

	_ = store.CreateWebAuthnCredential(ctx, CreateWebAuthnCredentialParams{
		UserID:       user.ID,
		CredentialID: []byte("cred"),
		PublicKey:    []byte("pk"),
		Transports:   []string{},
		Name:         "Original",
	})
	stored, _ := store.ListWebAuthnCredentialsByUserID(ctx, user.ID)
	credentialID := stored[0].ID

	if count, _ := service.CountPasskeys(ctx, user.ID); count != 1 {
		t.Fatalf("CountPasskeys() = %d, want 1", count)
	}

	if err := service.RenamePasskey(ctx, user.ID, credentialID, "Renamed"); err != nil {
		t.Fatalf("RenamePasskey() error = %v", err)
	}
	list, _ := service.ListPasskeys(ctx, user.ID)
	if list[0].Name != "Renamed" {
		t.Fatalf("name = %q, want Renamed", list[0].Name)
	}

	// Deleting the last passkey is allowed (password remains as fallback).
	if err := service.DeletePasskey(ctx, user.ID, credentialID); err != nil {
		t.Fatalf("DeletePasskey() error = %v", err)
	}
	if count, _ := service.CountPasskeys(ctx, user.ID); count != 0 {
		t.Fatalf("CountPasskeys() after delete = %d, want 0", count)
	}

	// Operating on a missing credential reports not found.
	if err := service.RenamePasskey(ctx, user.ID, credentialID, "x"); !errors.Is(err, ErrPasskeyNotFound) {
		t.Fatalf("RenamePasskey() error = %v, want %v", err, ErrPasskeyNotFound)
	}
	if err := service.DeletePasskey(ctx, user.ID, credentialID); !errors.Is(err, ErrPasskeyNotFound) {
		t.Fatalf("DeletePasskey() error = %v, want %v", err, ErrPasskeyNotFound)
	}
}
