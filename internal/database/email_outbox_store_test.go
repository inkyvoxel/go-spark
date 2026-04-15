package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

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
	if claimed[0].Status != "sending" {
		t.Fatalf("claimed status = %q, want sending", claimed[0].Status)
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
	sent, err := store.MarkSent(context.Background(), row.ID, sentAt)
	if err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	if sent.Status != "sent" {
		t.Fatalf("sent status = %q, want sent", sent.Status)
	}
	if !sent.SentAt.Valid {
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
	failed, err := store.MarkFailed(context.Background(), row.ID, "provider unavailable", retryAt)
	if err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	if failed.Status != "pending" {
		t.Fatalf("failed status = %q, want pending", failed.Status)
	}
	if failed.Attempts != 1 {
		t.Fatalf("failed attempts = %d, want 1", failed.Attempts)
	}
	if failed.LastError != "provider unavailable" {
		t.Fatalf("failed last error = %q, want provider unavailable", failed.LastError)
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

	if _, err := store.MarkSent(context.Background(), 999, time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkSent() error = %v, want %v", err, sql.ErrNoRows)
	}

	if _, err := store.MarkFailed(context.Background(), 999, "missing", time.Now().UTC()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkFailed() error = %v, want %v", err, sql.ErrNoRows)
	}
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
