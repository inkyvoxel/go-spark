package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
)

type EmailOutboxStore struct {
	queries *db.Queries
}

func NewEmailOutboxStore(conn *sql.DB) *EmailOutboxStore {
	return &EmailOutboxStore{queries: db.New(conn)}
}

func (s *EmailOutboxStore) Enqueue(ctx context.Context, message email.Message, availableAt time.Time) (db.EmailOutbox, error) {
	row, err := s.queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
		Sender:      message.From,
		Recipient:   message.To,
		Subject:     message.Subject,
		TextBody:    message.TextBody,
		HtmlBody:    message.HTMLBody,
		AvailableAt: availableAt,
	})
	if err != nil {
		return db.EmailOutbox{}, fmt.Errorf("enqueue email: %w", err)
	}

	return row, nil
}

func (s *EmailOutboxStore) ClaimPending(ctx context.Context, now time.Time, limit int64) ([]db.EmailOutbox, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("claim pending emails limit must be greater than zero")
	}

	rows, err := s.queries.ClaimPendingEmails(ctx, db.ClaimPendingEmailsParams{
		AvailableAt: now,
		Limit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("claim pending emails: %w", err)
	}

	return rows, nil
}

func (s *EmailOutboxStore) MarkSent(ctx context.Context, id int64, sentAt time.Time) (db.EmailOutbox, error) {
	row, err := s.queries.MarkEmailSent(ctx, db.MarkEmailSentParams{
		ID:     id,
		SentAt: sql.NullTime{Time: sentAt, Valid: true},
	})
	if err != nil {
		return db.EmailOutbox{}, fmt.Errorf("mark email sent: %w", err)
	}

	return row, nil
}

func (s *EmailOutboxStore) MarkFailed(ctx context.Context, id int64, lastError string, availableAt time.Time) (db.EmailOutbox, error) {
	row, err := s.queries.MarkEmailFailed(ctx, db.MarkEmailFailedParams{
		ID:          id,
		LastError:   lastError,
		AvailableAt: availableAt,
	})
	if err != nil {
		return db.EmailOutbox{}, fmt.Errorf("mark email failed: %w", err)
	}

	return row, nil
}
