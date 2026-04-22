package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
)

const emailOutboxTestSchema = `
CREATE TABLE email_outbox (
    id INTEGER PRIMARY KEY,
    sender TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    text_body TEXT NOT NULL,
    html_body TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    available_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    claimed_at TIMESTAMP,
    claim_expires_at TIMESTAMP,
    claim_token TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX email_outbox_pending_idx ON email_outbox(status, available_at);
CREATE INDEX email_outbox_claim_expiry_idx ON email_outbox(status, claim_expires_at);
`

func TestEmailOutboxStoreEnqueueAndClaimPending(t *testing.T) {
	store := newTestEmailOutboxStore(t)
	now := time.Now().UTC()

	ready, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "user@example.com",
		Subject:  "Ready",
		TextBody: "Ready text",
		HTMLBody: "<p>Ready</p>",
	}, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("Enqueue() ready error = %v", err)
	}

	if _, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "later@example.com",
		Subject:  "Later",
		TextBody: "Later text",
	}, now.Add(time.Hour)); err != nil {
		t.Fatalf("Enqueue() later error = %v", err)
	}

	claimed, err := store.ClaimPending(context.Background(), now, 2*time.Minute, 10)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed length = %d, want 1", len(claimed))
	}
	if claimed[0].ID != ready.ID {
		t.Fatalf("claimed ID = %d, want %d", claimed[0].ID, ready.ID)
	}
	if claimed[0].ClaimToken == "" {
		t.Fatal("ClaimToken is empty, want non-empty")
	}

	got := getEmailOutboxRow(t, store, ready.ID)
	if got.Status != "sending" {
		t.Fatalf("status = %q, want sending", got.Status)
	}
	if !got.ClaimedAt.Valid {
		t.Fatal("ClaimedAt.Valid = false, want true")
	}
	if !got.ClaimExpiresAt.Valid {
		t.Fatal("ClaimExpiresAt.Valid = false, want true")
	}
	if got.ClaimToken == "" {
		t.Fatal("ClaimToken = empty, want populated")
	}

	claimedAgain, err := store.ClaimPending(context.Background(), now, 2*time.Minute, 10)
	if err != nil {
		t.Fatalf("ClaimPending() second error = %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("claimed second length = %d, want 0", len(claimedAgain))
	}
}

func TestEmailOutboxStoreReclaimsExpiredSendingRows(t *testing.T) {
	store := newTestEmailOutboxStore(t)
	now := time.Now().UTC()

	row, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "user@example.com",
		Subject:  "Subject",
		TextBody: "Text",
	}, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if _, err := store.queries.MarkEmailFailed(context.Background(), db.MarkEmailFailedParams{
		ID:          row.ID,
		ClaimToken:  "",
		LastError:   "",
		AvailableAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("MarkEmailFailed() setup error = %v", err)
	}
	if _, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now, Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(-time.Second), Valid: true},
		ClaimToken:     "stale-token",
		Limit:          1,
	}); err != nil {
		t.Fatalf("ClaimPendingEmails() setup error = %v", err)
	}

	claimed, err := store.ClaimPending(context.Background(), now, 2*time.Minute, 1)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed length = %d, want 1", len(claimed))
	}
	if claimed[0].ID != row.ID {
		t.Fatalf("claimed ID = %d, want %d", claimed[0].ID, row.ID)
	}
	if claimed[0].ClaimToken == "stale-token" {
		t.Fatalf("ClaimToken = %q, want refreshed token", claimed[0].ClaimToken)
	}
}

func TestEmailOutboxStoreDoesNotReclaimActiveSendingRows(t *testing.T) {
	store := newTestEmailOutboxStore(t)
	now := time.Now().UTC()

	row, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "user@example.com",
		Subject:  "Subject",
		TextBody: "Text",
	}, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if _, err := store.queries.ClaimPendingEmails(context.Background(), db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now, Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(5 * time.Minute), Valid: true},
		ClaimToken:     "active-token",
		Limit:          1,
	}); err != nil {
		t.Fatalf("ClaimPendingEmails() setup error = %v", err)
	}

	claimed, err := store.ClaimPending(context.Background(), now.Add(time.Minute), 2*time.Minute, 1)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed = %#v, want none", claimed)
	}

	got := getEmailOutboxRow(t, store, row.ID)
	if got.ClaimToken != "active-token" {
		t.Fatalf("ClaimToken = %q, want active-token", got.ClaimToken)
	}
}

func TestEmailOutboxStoreMarkSent(t *testing.T) {
	store := newTestEmailOutboxStore(t)
	now := time.Now().UTC()

	row, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "user@example.com",
		Subject:  "Subject",
		TextBody: "Text",
	}, now)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	claimed, err := store.ClaimPending(context.Background(), now, 2*time.Minute, 1)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}

	sentAt := now.Add(time.Minute)
	if err := store.MarkSent(context.Background(), row.ID, claimed[0].ClaimToken, sentAt); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	got := getEmailOutboxRow(t, store, row.ID)
	if got.Status != "sent" {
		t.Fatalf("sent status = %q, want sent", got.Status)
	}
	if !got.SentAt.Valid {
		t.Fatal("SentAt.Valid = false, want true")
	}
	if got.ClaimToken != "" {
		t.Fatalf("ClaimToken = %q, want empty", got.ClaimToken)
	}
}

func TestEmailOutboxStoreMarkFailed(t *testing.T) {
	store := newTestEmailOutboxStore(t)
	now := time.Now().UTC()

	row, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "user@example.com",
		Subject:  "Subject",
		TextBody: "Text",
	}, now)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	claimed, err := store.ClaimPending(context.Background(), now, 2*time.Minute, 1)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}

	retryAt := now.Add(time.Minute)
	if err := store.MarkFailed(context.Background(), row.ID, claimed[0].ClaimToken, "provider unavailable", retryAt); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	got := getEmailOutboxRow(t, store, row.ID)
	if got.Status != "pending" {
		t.Fatalf("failed status = %q, want pending", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("failed attempts = %d, want 1", got.Attempts)
	}
	if got.LastError != "provider unavailable" {
		t.Fatalf("failed last error = %q, want provider unavailable", got.LastError)
	}
	if got.ClaimToken != "" {
		t.Fatalf("ClaimToken = %q, want empty", got.ClaimToken)
	}
}

func TestEmailOutboxStoreMarkFailedPermanently(t *testing.T) {
	store := newTestEmailOutboxStore(t)
	now := time.Now().UTC()

	row, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "user@example.com",
		Subject:  "Subject",
		TextBody: "Text",
	}, now)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	claimed, err := store.ClaimPending(context.Background(), now, 2*time.Minute, 1)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}

	if err := store.MarkFailedPermanently(context.Background(), row.ID, claimed[0].ClaimToken, "bad recipient", now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkFailedPermanently() error = %v", err)
	}
	got := getEmailOutboxRow(t, store, row.ID)
	if got.Status != "failed" {
		t.Fatalf("failed status = %q, want failed", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("failed attempts = %d, want 1", got.Attempts)
	}
	if got.LastError != "bad recipient" {
		t.Fatalf("failed last error = %q, want bad recipient", got.LastError)
	}
	if got.ClaimToken != "" {
		t.Fatalf("ClaimToken = %q, want empty", got.ClaimToken)
	}
}

func TestEmailOutboxStoreRejectsInvalidClaimLimit(t *testing.T) {
	store := newTestEmailOutboxStore(t)

	_, err := store.ClaimPending(context.Background(), time.Now().UTC(), 2*time.Minute, 0)
	if err == nil {
		t.Fatal("ClaimPending() error = nil, want error")
	}
}

func TestEmailOutboxStoreRejectsInvalidClaimTTL(t *testing.T) {
	store := newTestEmailOutboxStore(t)

	_, err := store.ClaimPending(context.Background(), time.Now().UTC(), 0, 1)
	if err == nil {
		t.Fatal("ClaimPending() error = nil, want error")
	}
}

func TestEmailOutboxStoreMissingRowsReturnNoRows(t *testing.T) {
	store := newTestEmailOutboxStore(t)

	if err := store.MarkSent(context.Background(), 999, "missing", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkSent() error = %v, want %v", err, sql.ErrNoRows)
	}

	if err := store.MarkFailed(context.Background(), 999, "missing", "missing", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkFailed() error = %v, want %v", err, sql.ErrNoRows)
	}

	if err := store.MarkFailedPermanently(context.Background(), 999, "missing", "missing", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkFailedPermanently() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func TestEmailOutboxStoreMarkOperationsRequireMatchingClaimToken(t *testing.T) {
	store := newTestEmailOutboxStore(t)
	now := time.Now().UTC()

	row, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "user@example.com",
		Subject:  "Subject",
		TextBody: "Text",
	}, now)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if _, err := store.ClaimPending(context.Background(), now, 2*time.Minute, 1); err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}

	if err := store.MarkSent(context.Background(), row.ID, "wrong-token", now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkSent() error = %v, want %v", err, sql.ErrNoRows)
	}
}

func getEmailOutboxRow(t *testing.T, store *EmailOutboxStore, id int64) db.EmailOutbox {
	t.Helper()

	row, err := store.queries.GetEmailOutboxRow(context.Background(), id)
	if err != nil {
		t.Fatalf("GetEmailOutboxRow() error = %v", err)
	}

	return row
}

func newTestEmailOutboxStore(t *testing.T) *EmailOutboxStore {
	t.Helper()

	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	if _, err := conn.Exec(emailOutboxTestSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return NewEmailOutboxStore(conn)
}
