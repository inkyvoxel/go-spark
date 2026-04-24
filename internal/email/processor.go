package email

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	DefaultWorkerBatchSize = 10
	DefaultRetryDelay      = time.Minute
	DefaultMaxAttempts     = 5
	DefaultClaimTTL        = 2 * time.Minute
	MaxLastErrorLength     = 1024
)

type OutboxEmail struct {
	ID         int64
	Message    Message
	Attempts   int64
	ClaimToken string
}

type OutboxStore interface {
	ClaimPending(ctx context.Context, now time.Time, claimTTL time.Duration, limit int64) ([]OutboxEmail, error)
	MarkSent(ctx context.Context, id int64, claimToken string, sentAt time.Time) error
	MarkFailed(ctx context.Context, id int64, claimToken, lastError string, availableAt time.Time) error
	MarkFailedPermanently(ctx context.Context, id int64, claimToken, lastError string, failedAt time.Time) error
}

type Processor struct {
	store       OutboxStore
	sender      Sender
	logger      *slog.Logger
	batchSize   int64
	retryDelay  time.Duration
	maxAttempts int64
	claimTTL    time.Duration
}

type ProcessorOptions struct {
	Logger      *slog.Logger
	BatchSize   int64
	RetryDelay  time.Duration
	MaxAttempts int64
	ClaimTTL    time.Duration
}

func NewProcessor(store OutboxStore, sender Sender, opts ProcessorOptions) *Processor {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	batchSize := opts.BatchSize
	if batchSize == 0 {
		batchSize = DefaultWorkerBatchSize
	}

	retryDelay := opts.RetryDelay
	if retryDelay == 0 {
		retryDelay = DefaultRetryDelay
	}

	maxAttempts := opts.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = DefaultMaxAttempts
	}

	claimTTL := opts.ClaimTTL
	if claimTTL == 0 {
		claimTTL = DefaultClaimTTL
	}

	return &Processor{
		store:       store,
		sender:      sender,
		logger:      logger,
		batchSize:   batchSize,
		retryDelay:  retryDelay,
		maxAttempts: maxAttempts,
		claimTTL:    claimTTL,
	}
}

func (p *Processor) Validate() error {
	if p.store == nil {
		return fmt.Errorf("email processor store is required")
	}
	if p.sender == nil {
		return fmt.Errorf("email processor sender is required")
	}
	return nil
}

func (p *Processor) ProcessPending(ctx context.Context) error {
	if err := p.Validate(); err != nil {
		return err
	}

	startedAt := time.Now().UTC()
	now := time.Now().UTC()
	messages, err := p.store.ClaimPending(ctx, now, p.claimTTL, p.batchSize)
	if err != nil {
		return fmt.Errorf("claim pending email: %w", err)
	}

	claimed := len(messages)
	sent := 0
	retryScheduled := 0
	permanentFailures := 0
	skippedClaimLost := 0

	for _, outboxEmail := range messages {
		if err := p.sender.Send(ctx, outboxEmail.Message); err != nil {
			nextAttempt := outboxEmail.Attempts + 1
			lastError := truncateLastError(err.Error())
			if nextAttempt >= p.maxAttempts {
				if markErr := p.store.MarkFailedPermanently(ctx, outboxEmail.ID, outboxEmail.ClaimToken, lastError, time.Now().UTC()); markErr != nil {
					if errors.Is(markErr, sql.ErrNoRows) {
						p.logger.Warn("email outbox claim no longer owned while marking permanent failure", "outbox_id", outboxEmail.ID)
						skippedClaimLost++
						continue
					}
					return fmt.Errorf("mark email permanently failed after send error %q: %w", err.Error(), markErr)
				}
				p.logger.Warn("email delivery failed permanently", "outbox_id", outboxEmail.ID, "attempts", nextAttempt, "err", err)
				permanentFailures++
				continue
			}

			if markErr := p.store.MarkFailed(ctx, outboxEmail.ID, outboxEmail.ClaimToken, lastError, time.Now().UTC().Add(p.retryDelay)); markErr != nil {
				if errors.Is(markErr, sql.ErrNoRows) {
					p.logger.Warn("email outbox claim no longer owned while marking retry", "outbox_id", outboxEmail.ID)
					skippedClaimLost++
					continue
				}
				return fmt.Errorf("mark email failed after send error %q: %w", err.Error(), markErr)
			}
			retryScheduled++
			continue
		}

		if err := p.store.MarkSent(ctx, outboxEmail.ID, outboxEmail.ClaimToken, time.Now().UTC()); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				p.logger.Warn("email outbox claim no longer owned while marking sent", "outbox_id", outboxEmail.ID)
				skippedClaimLost++
				continue
			}
			return fmt.Errorf("mark email sent: %w", err)
		}
		sent++
	}

	if claimed > 0 || retryScheduled > 0 || permanentFailures > 0 || skippedClaimLost > 0 {
		p.logger.Info(
			"email outbox cycle completed",
			"claimed", claimed,
			"sent", sent,
			"retry_scheduled", retryScheduled,
			"permanent_failures", permanentFailures,
			"skipped_claim_lost", skippedClaimLost,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}

	return nil
}

func truncateLastError(lastError string) string {
	if len(lastError) <= MaxLastErrorLength {
		return lastError
	}
	return lastError[:MaxLastErrorLength]
}
