package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
)

func TestCleanupStorePrunesExpiredSessionsAndTokensAndOldEmails(t *testing.T) {
	conn := newAuthStoreTestDB(t)
	store := NewCleanupStore(conn)
	queries := db.New(conn)

	user, err := queries.CreateUser(context.Background(), db.CreateUserParams{
		Email:        "user@example.com",
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	now := time.Now().UTC()

	expiredSession, err := queries.CreateSession(context.Background(), db.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: "expired-session",
		ExpiresAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateSession() expired error = %v", err)
	}
	activeSession, err := queries.CreateSession(context.Background(), db.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: "active-session",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession() active error = %v", err)
	}

	if _, err := queries.CreatePasswordResetToken(context.Background(), db.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: "reset-expired",
		ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreatePasswordResetToken() expired error = %v", err)
	}
	oldConsumedReset, err := queries.CreatePasswordResetToken(context.Background(), db.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: "reset-old-consumed",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreatePasswordResetToken() old consumed error = %v", err)
	}
	if _, err := queries.ConsumePasswordResetToken(context.Background(), db.ConsumePasswordResetTokenParams{
		ConsumedAt: sql.NullTime{Time: now.Add(-48 * time.Hour), Valid: true},
		TokenHash:  oldConsumedReset.TokenHash,
		ExpiresAt:  now.Add(-47 * time.Hour),
	}); err != nil {
		t.Fatalf("ConsumePasswordResetToken() old consumed error = %v", err)
	}
	recentConsumedReset, err := queries.CreatePasswordResetToken(context.Background(), db.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: "reset-recent-consumed",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreatePasswordResetToken() recent consumed error = %v", err)
	}
	if _, err := queries.ConsumePasswordResetToken(context.Background(), db.ConsumePasswordResetTokenParams{
		ConsumedAt: sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
		TokenHash:  recentConsumedReset.TokenHash,
		ExpiresAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("ConsumePasswordResetToken() recent consumed error = %v", err)
	}

	if _, err := queries.CreateEmailVerificationToken(context.Background(), db.CreateEmailVerificationTokenParams{
		UserID:    user.ID,
		TokenHash: "verify-expired",
		ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreateEmailVerificationToken() expired error = %v", err)
	}
	oldConsumedVerification, err := queries.CreateEmailVerificationToken(context.Background(), db.CreateEmailVerificationTokenParams{
		UserID:    user.ID,
		TokenHash: "verify-old-consumed",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken() old consumed error = %v", err)
	}
	if _, err := queries.ConsumeEmailVerificationToken(context.Background(), db.ConsumeEmailVerificationTokenParams{
		ConsumedAt: sql.NullTime{Time: now.Add(-48 * time.Hour), Valid: true},
		TokenHash:  oldConsumedVerification.TokenHash,
		ExpiresAt:  now.Add(-47 * time.Hour),
	}); err != nil {
		t.Fatalf("ConsumeEmailVerificationToken() old consumed error = %v", err)
	}
	recentConsumedVerification, err := queries.CreateEmailVerificationToken(context.Background(), db.CreateEmailVerificationTokenParams{
		UserID:    user.ID,
		TokenHash: "verify-recent-consumed",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken() recent consumed error = %v", err)
	}
	if _, err := queries.ConsumeEmailVerificationToken(context.Background(), db.ConsumeEmailVerificationTokenParams{
		ConsumedAt: sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
		TokenHash:  recentConsumedVerification.TokenHash,
		ExpiresAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("ConsumeEmailVerificationToken() recent consumed error = %v", err)
	}

	if _, err := queries.CreateEmailChangeToken(context.Background(), db.CreateEmailChangeTokenParams{
		UserID:    user.ID,
		NewEmail:  "expired-change@example.com",
		TokenHash: "change-expired",
		ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreateEmailChangeToken() expired error = %v", err)
	}
	oldConsumedChange, err := queries.CreateEmailChangeToken(context.Background(), db.CreateEmailChangeTokenParams{
		UserID:    user.ID,
		NewEmail:  "old-change@example.com",
		TokenHash: "change-old-consumed",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateEmailChangeToken() old consumed error = %v", err)
	}
	if _, err := queries.ConsumeEmailChangeToken(context.Background(), db.ConsumeEmailChangeTokenParams{
		ConsumedAt: sql.NullTime{Time: now.Add(-48 * time.Hour), Valid: true},
		TokenHash:  oldConsumedChange.TokenHash,
		ExpiresAt:  now.Add(-47 * time.Hour),
	}); err != nil {
		t.Fatalf("ConsumeEmailChangeToken() old consumed error = %v", err)
	}
	recentConsumedChange, err := queries.CreateEmailChangeToken(context.Background(), db.CreateEmailChangeTokenParams{
		UserID:    user.ID,
		NewEmail:  "recent-change@example.com",
		TokenHash: "change-recent-consumed",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateEmailChangeToken() recent consumed error = %v", err)
	}
	if _, err := queries.ConsumeEmailChangeToken(context.Background(), db.ConsumeEmailChangeTokenParams{
		ConsumedAt: sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
		TokenHash:  recentConsumedChange.TokenHash,
		ExpiresAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("ConsumeEmailChangeToken() recent consumed error = %v", err)
	}

	oldSent, err := queries.EnqueueEmail(context.Background(), db.EnqueueEmailParams{
		Sender:      "sender@example.com",
		Recipient:   "sent-old@example.com",
		Subject:     "old sent",
		TextBody:    "old sent",
		HtmlBody:    "",
		AvailableAt: now.Add(-10 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("EnqueueEmail() old sent error = %v", err)
	}
	if _, err := queries.MarkEmailSent(context.Background(), db.MarkEmailSentParams{
		ID:         oldSent.ID,
		ClaimToken: "",
		SentAt:     sql.NullTime{Time: now.Add(-8 * 24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("MarkEmailSent() old sent error = %v", err)
	}
	recentSent, err := queries.EnqueueEmail(context.Background(), db.EnqueueEmailParams{
		Sender:      "sender@example.com",
		Recipient:   "sent-recent@example.com",
		Subject:     "recent sent",
		TextBody:    "recent sent",
		HtmlBody:    "",
		AvailableAt: now.Add(-2 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("EnqueueEmail() recent sent error = %v", err)
	}
	if _, err := queries.MarkEmailSent(context.Background(), db.MarkEmailSentParams{
		ID:         recentSent.ID,
		ClaimToken: "",
		SentAt:     sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("MarkEmailSent() recent sent error = %v", err)
	}
	oldFailed, err := queries.EnqueueEmail(context.Background(), db.EnqueueEmailParams{
		Sender:      "sender@example.com",
		Recipient:   "failed-old@example.com",
		Subject:     "old failed",
		TextBody:    "old failed",
		HtmlBody:    "",
		AvailableAt: now.Add(-20 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("EnqueueEmail() old failed error = %v", err)
	}
	if _, err := queries.MarkEmailFailedPermanently(context.Background(), db.MarkEmailFailedPermanentlyParams{
		ID:          oldFailed.ID,
		ClaimToken:  "",
		LastError:   "old failed",
		AvailableAt: now.Add(-15 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("MarkEmailFailedPermanently() old failed error = %v", err)
	}
	recentFailed, err := queries.EnqueueEmail(context.Background(), db.EnqueueEmailParams{
		Sender:      "sender@example.com",
		Recipient:   "failed-recent@example.com",
		Subject:     "recent failed",
		TextBody:    "recent failed",
		HtmlBody:    "",
		AvailableAt: now.Add(-5 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("EnqueueEmail() recent failed error = %v", err)
	}
	if _, err := queries.MarkEmailFailedPermanently(context.Background(), db.MarkEmailFailedPermanentlyParams{
		ID:          recentFailed.ID,
		ClaimToken:  "",
		LastError:   "recent failed",
		AvailableAt: now.Add(-7 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("MarkEmailFailedPermanently() recent failed error = %v", err)
	}
	pendingRow, err := queries.EnqueueEmail(context.Background(), db.EnqueueEmailParams{
		Sender:      "sender@example.com",
		Recipient:   "pending@example.com",
		Subject:     "pending",
		TextBody:    "pending",
		HtmlBody:    "",
		AvailableAt: now.Add(-30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("EnqueueEmail() pending error = %v", err)
	}
	sendingRow, err := queries.EnqueueEmail(context.Background(), db.EnqueueEmailParams{
		Sender:      "sender@example.com",
		Recipient:   "sending@example.com",
		Subject:     "sending",
		TextBody:    "sending",
		HtmlBody:    "",
		AvailableAt: now.Add(-30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("EnqueueEmail() sending error = %v", err)
	}
	if _, err := queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now, Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(time.Hour), Valid: true},
		ClaimToken:     "claim-token",
		Limit:          2,
	}); err != nil {
		t.Fatalf("ClaimPendingEmails() error = %v", err)
	}

	deletedSessions, err := store.DeleteExpiredSessions(context.Background(), now)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions() error = %v", err)
	}
	if deletedSessions != 1 {
		t.Fatalf("DeleteExpiredSessions() = %d, want 1", deletedSessions)
	}

	deletedResetTokens, err := store.PrunePasswordResetTokens(context.Background(), now, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("PrunePasswordResetTokens() error = %v", err)
	}
	if deletedResetTokens != 2 {
		t.Fatalf("PrunePasswordResetTokens() = %d, want 2", deletedResetTokens)
	}

	deletedVerificationTokens, err := store.PruneEmailVerificationTokens(context.Background(), now, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("PruneEmailVerificationTokens() error = %v", err)
	}
	if deletedVerificationTokens != 2 {
		t.Fatalf("PruneEmailVerificationTokens() = %d, want 2", deletedVerificationTokens)
	}

	deletedEmailChangeTokens, err := store.PruneEmailChangeTokens(context.Background(), now, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("PruneEmailChangeTokens() error = %v", err)
	}
	if deletedEmailChangeTokens != 2 {
		t.Fatalf("PruneEmailChangeTokens() = %d, want 2", deletedEmailChangeTokens)
	}

	deletedSentRows, err := store.PruneSentEmailOutboxRows(context.Background(), now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("PruneSentEmailOutboxRows() error = %v", err)
	}
	if deletedSentRows != 1 {
		t.Fatalf("PruneSentEmailOutboxRows() = %d, want 1", deletedSentRows)
	}

	deletedFailedRows, err := store.PruneFailedEmailOutboxRows(context.Background(), now.Add(-14*24*time.Hour))
	if err != nil {
		t.Fatalf("PruneFailedEmailOutboxRows() error = %v", err)
	}
	if deletedFailedRows != 1 {
		t.Fatalf("PruneFailedEmailOutboxRows() = %d, want 1", deletedFailedRows)
	}

	assertRowCount(t, conn, "SELECT COUNT(*) FROM sessions WHERE id = ?", activeSession.ID, 1)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM sessions WHERE id = ?", expiredSession.ID, 0)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM password_reset_tokens WHERE token_hash = ?", "reset-recent-consumed", 1)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM password_reset_tokens WHERE token_hash = ?", "reset-expired", 0)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_verification_tokens WHERE token_hash = ?", "verify-recent-consumed", 1)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_verification_tokens WHERE token_hash = ?", "verify-expired", 0)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_change_tokens WHERE token_hash = ?", "change-recent-consumed", 1)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_change_tokens WHERE token_hash = ?", "change-expired", 0)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_outbox WHERE id = ?", oldSent.ID, 0)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_outbox WHERE id = ?", recentSent.ID, 1)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_outbox WHERE id = ?", oldFailed.ID, 0)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_outbox WHERE id = ?", recentFailed.ID, 1)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_outbox WHERE id = ?", pendingRow.ID, 1)
	assertRowCount(t, conn, "SELECT COUNT(*) FROM email_outbox WHERE id = ?", sendingRow.ID, 1)
}

func assertRowCount(t *testing.T, conn *sql.DB, query string, arg any, want int) {
	t.Helper()

	var count int
	if err := conn.QueryRowContext(context.Background(), query, arg).Scan(&count); err != nil {
		t.Fatalf("QueryRowContext() error = %v", err)
	}
	if count != want {
		t.Fatalf("count for %q with arg %v = %d, want %d", query, arg, count, want)
	}
}
