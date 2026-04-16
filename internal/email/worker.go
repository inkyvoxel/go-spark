package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	DefaultWorkerInterval  = 5 * time.Second
	DefaultWorkerBatchSize = 10
	DefaultRetryDelay      = time.Minute
	DefaultMaxAttempts     = 5
)

type OutboxEmail struct {
	ID       int64
	Message  Message
	Attempts int64
}

type OutboxStore interface {
	ClaimPending(ctx context.Context, now time.Time, limit int64) ([]OutboxEmail, error)
	MarkSent(ctx context.Context, id int64, sentAt time.Time) error
	MarkFailed(ctx context.Context, id int64, lastError string, availableAt time.Time) error
	MarkFailedPermanently(ctx context.Context, id int64, lastError string, failedAt time.Time) error
}

type Worker struct {
	store       OutboxStore
	sender      Sender
	logger      *slog.Logger
	interval    time.Duration
	batchSize   int64
	retryDelay  time.Duration
	maxAttempts int64
}

type WorkerOptions struct {
	Logger      *slog.Logger
	Interval    time.Duration
	BatchSize   int64
	RetryDelay  time.Duration
	MaxAttempts int64
}

func NewWorker(store OutboxStore, sender Sender, opts WorkerOptions) *Worker {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	interval := opts.Interval
	if interval == 0 {
		interval = DefaultWorkerInterval
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

	return &Worker{
		store:       store,
		sender:      sender,
		logger:      logger,
		interval:    interval,
		batchSize:   batchSize,
		retryDelay:  retryDelay,
		maxAttempts: maxAttempts,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.store == nil {
		return fmt.Errorf("email worker store is required")
	}
	if w.sender == nil {
		return fmt.Errorf("email worker sender is required")
	}

	if err := w.ProcessPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Error("process pending email", "err", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.ProcessPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("process pending email", "err", err)
			}
		}
	}
}

func (w *Worker) ProcessPending(ctx context.Context) error {
	messages, err := w.store.ClaimPending(ctx, time.Now().UTC(), w.batchSize)
	if err != nil {
		return fmt.Errorf("claim pending email: %w", err)
	}

	for _, outboxEmail := range messages {
		if err := w.sender.Send(ctx, outboxEmail.Message); err != nil {
			nextAttempt := outboxEmail.Attempts + 1
			if nextAttempt >= w.maxAttempts {
				if markErr := w.store.MarkFailedPermanently(ctx, outboxEmail.ID, err.Error(), time.Now().UTC()); markErr != nil {
					return fmt.Errorf("mark email permanently failed after send error %q: %w", err.Error(), markErr)
				}
				w.logger.Warn("email delivery failed permanently", "outbox_id", outboxEmail.ID, "attempts", nextAttempt, "err", err)
				continue
			}

			if markErr := w.store.MarkFailed(ctx, outboxEmail.ID, err.Error(), time.Now().UTC().Add(w.retryDelay)); markErr != nil {
				return fmt.Errorf("mark email failed after send error %q: %w", err.Error(), markErr)
			}
			continue
		}

		if err := w.store.MarkSent(ctx, outboxEmail.ID, time.Now().UTC()); err != nil {
			return fmt.Errorf("mark email sent: %w", err)
		}
	}

	return nil
}
