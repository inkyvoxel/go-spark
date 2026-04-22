package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
)

type EmailOutboxStore struct {
	queries *db.Queries
}

var _ email.OutboxStore = (*EmailOutboxStore)(nil)

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

func (s *EmailOutboxStore) ClaimPending(ctx context.Context, now time.Time, claimTTL time.Duration, limit int64) ([]email.OutboxEmail, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("claim pending emails limit must be greater than zero")
	}
	if claimTTL <= 0 {
		return nil, fmt.Errorf("claim pending emails ttl must be greater than zero")
	}

	claimToken, err := generateClaimToken()
	if err != nil {
		return nil, fmt.Errorf("generate outbox claim token: %w", err)
	}

	rows, err := s.queries.ClaimPendingEmails(ctx, db.ClaimPendingEmailsParams{
		Now:            sql.NullTime{Time: now, Valid: true},
		ClaimExpiresAt: sql.NullTime{Time: now.Add(claimTTL), Valid: true},
		ClaimToken:     claimToken,
		Limit:          limit,
	})
	if err != nil {
		return nil, fmt.Errorf("claim pending emails: %w", err)
	}

	messages := make([]email.OutboxEmail, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, email.OutboxEmail{
			ID: row.ID,
			Message: email.Message{
				From:     row.Sender,
				To:       row.Recipient,
				Subject:  row.Subject,
				TextBody: row.TextBody,
				HTMLBody: row.HtmlBody,
			},
			Attempts:   row.Attempts,
			ClaimToken: row.ClaimToken,
		})
	}

	return messages, nil
}

func (s *EmailOutboxStore) MarkSent(ctx context.Context, id int64, claimToken string, sentAt time.Time) error {
	if _, err := s.queries.MarkEmailSent(ctx, db.MarkEmailSentParams{
		ID:         id,
		ClaimToken: claimToken,
		SentAt:     sql.NullTime{Time: sentAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("mark email sent: %w", err)
	}

	return nil
}

func (s *EmailOutboxStore) MarkFailed(ctx context.Context, id int64, claimToken, lastError string, availableAt time.Time) error {
	if _, err := s.queries.MarkEmailFailed(ctx, db.MarkEmailFailedParams{
		ID:          id,
		ClaimToken:  claimToken,
		LastError:   lastError,
		AvailableAt: availableAt,
	}); err != nil {
		return fmt.Errorf("mark email failed: %w", err)
	}

	return nil
}

func (s *EmailOutboxStore) MarkFailedPermanently(ctx context.Context, id int64, claimToken, lastError string, failedAt time.Time) error {
	if _, err := s.queries.MarkEmailFailedPermanently(ctx, db.MarkEmailFailedPermanentlyParams{
		ID:          id,
		ClaimToken:  claimToken,
		LastError:   lastError,
		AvailableAt: failedAt,
	}); err != nil {
		return fmt.Errorf("mark email permanently failed: %w", err)
	}

	return nil
}

func generateClaimToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
