package services

import (
	"database/sql"
	"testing"
	"time"
)

func TestRequiredEmailVerificationPolicyUsesPersistedVerificationState(t *testing.T) {
	policy := NewEmailVerificationPolicy(true)
	if !policy.RequiresEmailChangeVerification() {
		t.Fatal("RequiresEmailChangeVerification() = false, want true")
	}

	if policy.UserIsVerified(User{ID: 1, Email: "user@example.com"}) {
		t.Fatal("UserIsVerified() = true, want false for unverified user")
	}
	if !policy.UserIsVerified(User{ID: 1, Email: "user@example.com", EmailVerifiedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}}) {
		t.Fatal("UserIsVerified() = false, want true for verified user")
	}
}

func TestOptionalEmailVerificationPolicyTreatsAllUsersAsVerified(t *testing.T) {
	policy := NewEmailVerificationPolicy(false)
	if policy.RequiresEmailChangeVerification() {
		t.Fatal("RequiresEmailChangeVerification() = true, want false")
	}

	if !policy.UserIsVerified(User{ID: 1, Email: "user@example.com"}) {
		t.Fatal("UserIsVerified() = false, want true for unverified user in optional mode")
	}
}
