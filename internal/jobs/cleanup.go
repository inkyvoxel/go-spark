package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const DefaultCleanupInterval = time.Hour

type CleanupStore interface {
	DeleteExpiredSessions(ctx context.Context, expiredBefore time.Time) (int64, error)
	PrunePasswordResetTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error)
	PruneEmailVerificationTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error)
	PruneEmailChangeTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error)
	PruneSentEmailOutboxRows(ctx context.Context, sentBefore time.Time) (int64, error)
	PruneFailedEmailOutboxRows(ctx context.Context, failedBefore time.Time) (int64, error)
}

type CleanupOptions struct {
	Logger               *slog.Logger
	TokenRetention       time.Duration
	SentEmailRetention   time.Duration
	FailedEmailRetention time.Duration
}

type CleanupJob struct {
	store                CleanupStore
	logger               *slog.Logger
	tokenRetention       time.Duration
	sentEmailRetention   time.Duration
	failedEmailRetention time.Duration
}

func NewCleanupJob(store CleanupStore, opts CleanupOptions) (*CleanupJob, error) {
	if store == nil {
		return nil, fmt.Errorf("cleanup store is required")
	}
	if opts.TokenRetention <= 0 {
		return nil, fmt.Errorf("cleanup token retention must be greater than zero")
	}
	if opts.SentEmailRetention <= 0 {
		return nil, fmt.Errorf("cleanup sent email retention must be greater than zero")
	}
	if opts.FailedEmailRetention <= 0 {
		return nil, fmt.Errorf("cleanup failed email retention must be greater than zero")
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &CleanupJob{
		store:                store,
		logger:               logger,
		tokenRetention:       opts.TokenRetention,
		sentEmailRetention:   opts.SentEmailRetention,
		failedEmailRetention: opts.FailedEmailRetention,
	}, nil
}

func (j *CleanupJob) Job(interval time.Duration) Job {
	return Job{
		Name:       "database-cleanup",
		Interval:   interval,
		RunAtStart: true,
		Run:        j.Run,
	}
}

func (j *CleanupJob) Run(ctx context.Context) error {
	now := time.Now().UTC()
	tokenConsumedBefore := now.Add(-j.tokenRetention)
	sentBefore := now.Add(-j.sentEmailRetention)
	failedBefore := now.Add(-j.failedEmailRetention)

	var total int64

	deletedSessions, err := j.store.DeleteExpiredSessions(ctx, now)
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	total += deletedSessions

	deletedResetTokens, err := j.store.PrunePasswordResetTokens(ctx, now, tokenConsumedBefore)
	if err != nil {
		return fmt.Errorf("prune password reset tokens: %w", err)
	}
	total += deletedResetTokens

	deletedVerificationTokens, err := j.store.PruneEmailVerificationTokens(ctx, now, tokenConsumedBefore)
	if err != nil {
		return fmt.Errorf("prune email verification tokens: %w", err)
	}
	total += deletedVerificationTokens

	deletedEmailChangeTokens, err := j.store.PruneEmailChangeTokens(ctx, now, tokenConsumedBefore)
	if err != nil {
		return fmt.Errorf("prune email change tokens: %w", err)
	}
	total += deletedEmailChangeTokens

	deletedSentEmails, err := j.store.PruneSentEmailOutboxRows(ctx, sentBefore)
	if err != nil {
		return fmt.Errorf("prune sent email rows: %w", err)
	}
	total += deletedSentEmails

	deletedFailedEmails, err := j.store.PruneFailedEmailOutboxRows(ctx, failedBefore)
	if err != nil {
		return fmt.Errorf("prune failed email rows: %w", err)
	}
	total += deletedFailedEmails

	j.logger.Info(
		"database cleanup completed",
		"deleted_total", total,
		"expired_sessions", deletedSessions,
		"password_reset_tokens", deletedResetTokens,
		"email_verification_tokens", deletedVerificationTokens,
		"email_change_tokens", deletedEmailChangeTokens,
		"sent_emails", deletedSentEmails,
		"failed_emails", deletedFailedEmails,
	)

	return nil
}
