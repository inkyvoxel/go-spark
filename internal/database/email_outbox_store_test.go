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
    sent_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX email_outbox_pending_idx ON email_outbox(status, available_at);
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
	if ready.Status != "pending" {
		t.Fatalf("ready status = %q, want pending", ready.Status)
	}
	if ready.Sender != "sender@example.com" {
		t.Fatalf("ready sender = %q, want sender@example.com", ready.Sender)
	}

	if _, err := store.Enqueue(context.Background(), email.Message{
		From:     "sender@example.com",
		To:       "later@example.com",
		Subject:  "Later",
		TextBody: "Later text",
	}, now.Add(time.Hour)); err != nil {
		t.Fatalf("Enqueue() later error = %v", err)
	}

	claimed, err := store.ClaimPending(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed length = %d, want 1", len(claimed))
	}
	if claimed[0].ID != ready.ID {
		t.Fatalf("claimed ID = %d, want %d", claimed[0].ID, ready.ID)
	}
	if claimed[0].Message.From != "sender@example.com" {
		t.Fatalf("claimed from = %q, want sender@example.com", claimed[0].Message.From)
	}
	if claimed[0].Message.To != "user@example.com" {
		t.Fatalf("claimed to = %q, want user@example.com", claimed[0].Message.To)
	}

	claimedAgain, err := store.ClaimPending(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("ClaimPending() second error = %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("claimed second length = %d, want 0", len(claimedAgain))
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

	sentAt := now.Add(time.Minute)
	if err := store.MarkSent(context.Background(), row.ID, sentAt); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	got := getEmailOutboxRow(t, store, row.ID)
	if got.Status != "sent" {
		t.Fatalf("sent status = %q, want sent", got.Status)
	}
	if !got.SentAt.Valid {
		t.Fatal("SentAt.Valid = false, want true")
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
	if _, err := store.ClaimPending(context.Background(), now, 1); err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}

	retryAt := now.Add(time.Minute)
	if err := store.MarkFailed(context.Background(), row.ID, "provider unavailable", retryAt); err != nil {
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
	if _, err := store.ClaimPending(context.Background(), now, 1); err != nil {
		t.Fatalf("ClaimPending() error = %v", err)
	}

	if err := store.MarkFailedPermanently(context.Background(), row.ID, "bad recipient", now.Add(time.Minute)); err != nil {
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
}

func TestEmailOutboxStoreRejectsInvalidClaimLimit(t *testing.T) {
	store := newTestEmailOutboxStore(t)

	_, err := store.ClaimPending(context.Background(), time.Now().UTC(), 0)
	if err == nil {
		t.Fatal("ClaimPending() error = nil, want error")
	}
}

func TestEmailOutboxStoreMissingRowsReturnNoRows(t *testing.T) {
	store := newTestEmailOutboxStore(t)

	if err := store.MarkSent(context.Background(), 999, time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkSent() error = %v, want %v", err, sql.ErrNoRows)
	}

	if err := store.MarkFailed(context.Background(), 999, "missing", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkFailed() error = %v, want %v", err, sql.ErrNoRows)
	}

	if err := store.MarkFailedPermanently(context.Background(), 999, "missing", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkFailedPermanently() error = %v, want %v", err, sql.ErrNoRows)
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
