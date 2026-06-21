package services

import (
	"bytes"
	"context"
	"testing"
)

func TestWebAuthnCredentialLifecycle(t *testing.T) {
	store := newTestauthStore(t)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	handle, err := store.GetWebAuthnHandleByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetWebAuthnHandleByUserID() error = %v", err)
	}
	if len(handle) != webAuthnUserHandleBytes {
		t.Fatalf("handle length = %d, want %d", len(handle), webAuthnUserHandleBytes)
	}

	resolved, err := store.GetUserByWebAuthnHandle(ctx, handle)
	if err != nil {
		t.Fatalf("GetUserByWebAuthnHandle() error = %v", err)
	}
	if resolved.ID != user.ID {
		t.Fatalf("resolved user ID = %d, want %d", resolved.ID, user.ID)
	}

	params := CreateWebAuthnCredentialParams{
		UserID:          user.ID,
		CredentialID:    []byte("credential-one"),
		PublicKey:       []byte("public-key"),
		AttestationType: "none",
		AAGUID:          []byte("aaguid"),
		SignCount:       0,
		Transports:      []string{"internal", "hybrid"},
		BackupEligible:  true,
		BackupState:     true,
		Name:            "My Laptop",
	}
	if err := store.CreateWebAuthnCredential(ctx, params); err != nil {
		t.Fatalf("CreateWebAuthnCredential() error = %v", err)
	}

	credentials, err := store.ListWebAuthnCredentialsByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListWebAuthnCredentialsByUserID() error = %v", err)
	}
	if len(credentials) != 1 {
		t.Fatalf("len(credentials) = %d, want 1", len(credentials))
	}
	got := credentials[0]
	if !bytes.Equal(got.CredentialID, params.CredentialID) {
		t.Fatalf("CredentialID = %q, want %q", got.CredentialID, params.CredentialID)
	}
	if len(got.Transports) != 2 || got.Transports[0] != "internal" || got.Transports[1] != "hybrid" {
		t.Fatalf("Transports = %v, want [internal hybrid]", got.Transports)
	}
	if !got.BackupEligible || !got.BackupState {
		t.Fatalf("backup flags = (%v,%v), want (true,true)", got.BackupEligible, got.BackupState)
	}

	if err := store.UpdateWebAuthnCredentialOnLogin(ctx, UpdateWebAuthnCredentialParams{
		CredentialID: params.CredentialID,
		SignCount:    7,
		BackupState:  false,
		LastUsedAt:   got.CreatedAt,
	}); err != nil {
		t.Fatalf("UpdateWebAuthnCredentialOnLogin() error = %v", err)
	}
	credentials, _ = store.ListWebAuthnCredentialsByUserID(ctx, user.ID)
	if credentials[0].SignCount != 7 || credentials[0].BackupState {
		t.Fatalf("after update: sign_count=%d backup_state=%v, want 7,false", credentials[0].SignCount, credentials[0].BackupState)
	}
	if !credentials[0].LastUsedAt.Valid {
		t.Fatalf("LastUsedAt should be set after login update")
	}
}

func TestWebAuthnCredentialUniqueByCredentialID(t *testing.T) {
	store := newTestauthStore(t)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "user@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	params := CreateWebAuthnCredentialParams{
		UserID:       user.ID,
		CredentialID: []byte("duplicate"),
		PublicKey:    []byte("pk"),
		Transports:   []string{},
	}
	if err := store.CreateWebAuthnCredential(ctx, params); err != nil {
		t.Fatalf("first CreateWebAuthnCredential() error = %v", err)
	}
	if err := store.CreateWebAuthnCredential(ctx, params); err == nil {
		t.Fatal("second CreateWebAuthnCredential() with same credential ID should fail")
	}
}

func TestWebAuthnCredentialRenameAndDeleteAreScopedToUser(t *testing.T) {
	store := newTestauthStore(t)
	ctx := context.Background()

	owner, _ := store.CreateUser(ctx, "owner@example.com", "hash")
	other, _ := store.CreateUser(ctx, "other@example.com", "hash")

	if err := store.CreateWebAuthnCredential(ctx, CreateWebAuthnCredentialParams{
		UserID:       owner.ID,
		CredentialID: []byte("owned"),
		PublicKey:    []byte("pk"),
		Transports:   []string{},
		Name:         "Original",
	}); err != nil {
		t.Fatalf("CreateWebAuthnCredential() error = %v", err)
	}
	owned, _ := store.ListWebAuthnCredentialsByUserID(ctx, owner.ID)
	credentialID := owned[0].ID

	// Another user cannot rename or delete it.
	if renamed, _ := store.RenameWebAuthnCredential(ctx, other.ID, credentialID, "Hacked"); renamed {
		t.Fatal("RenameWebAuthnCredential() should not affect another user's credential")
	}
	if deleted, _ := store.DeleteWebAuthnCredential(ctx, other.ID, credentialID); deleted {
		t.Fatal("DeleteWebAuthnCredential() should not affect another user's credential")
	}

	// The owner can.
	if renamed, _ := store.RenameWebAuthnCredential(ctx, owner.ID, credentialID, "Renamed"); !renamed {
		t.Fatal("owner RenameWebAuthnCredential() should succeed")
	}
	count, _ := store.CountWebAuthnCredentialsByUserID(ctx, owner.ID)
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if deleted, _ := store.DeleteWebAuthnCredential(ctx, owner.ID, credentialID); !deleted {
		t.Fatal("owner DeleteWebAuthnCredential() should succeed")
	}
	count, _ = store.CountWebAuthnCredentialsByUserID(ctx, owner.ID)
	if count != 0 {
		t.Fatalf("count after delete = %d, want 0", count)
	}
}
