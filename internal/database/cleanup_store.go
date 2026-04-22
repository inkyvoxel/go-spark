package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/jobs"
)

type CleanupStore struct {
	queries *db.Queries
}

var _ jobs.CleanupStore = (*CleanupStore)(nil)

func NewCleanupStore(conn *sql.DB) *CleanupStore {
	return &CleanupStore{queries: db.New(conn)}
}

func (s *CleanupStore) DeleteExpiredSessions(ctx context.Context, expiredBefore time.Time) (int64, error) {
	rows, err := s.queries.DeleteExpiredSessions(ctx, expiredBefore)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return rows, nil
}

func (s *CleanupStore) PrunePasswordResetTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	rows, err := s.queries.PrunePasswordResetTokens(ctx, db.PrunePasswordResetTokensParams{
		ExpiredBefore:  expiredBefore,
		ConsumedBefore: sql.NullTime{Time: consumedBefore, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("prune password reset tokens: %w", err)
	}
	return rows, nil
}

func (s *CleanupStore) PruneEmailVerificationTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	rows, err := s.queries.PruneEmailVerificationTokens(ctx, db.PruneEmailVerificationTokensParams{
		ExpiredBefore:  expiredBefore,
		ConsumedBefore: sql.NullTime{Time: consumedBefore, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("prune email verification tokens: %w", err)
	}
	return rows, nil
}

func (s *CleanupStore) PruneEmailChangeTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	rows, err := s.queries.PruneEmailChangeTokens(ctx, db.PruneEmailChangeTokensParams{
		ExpiredBefore:  expiredBefore,
		ConsumedBefore: sql.NullTime{Time: consumedBefore, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("prune email change tokens: %w", err)
	}
	return rows, nil
}

func (s *CleanupStore) PruneSentEmailOutboxRows(ctx context.Context, sentBefore time.Time) (int64, error) {
	rows, err := s.queries.PruneSentEmailOutboxRows(ctx, sql.NullTime{Time: sentBefore, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("prune sent email outbox rows: %w", err)
	}
	return rows, nil
}

func (s *CleanupStore) PruneFailedEmailOutboxRows(ctx context.Context, failedBefore time.Time) (int64, error) {
	rows, err := s.queries.PruneFailedEmailOutboxRows(ctx, failedBefore)
	if err != nil {
		return 0, fmt.Errorf("prune failed email outbox rows: %w", err)
	}
	return rows, nil
}
