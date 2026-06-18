package database

import (
	"context"
	"database/sql"
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
	return s.queries.DeleteExpiredSessions(ctx, expiredBefore)
}

func (s *CleanupStore) PrunePasswordResetTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	return s.queries.PrunePasswordResetTokens(ctx, db.PrunePasswordResetTokensParams{
		ExpiredBefore:  expiredBefore,
		ConsumedBefore: sql.NullTime{Time: consumedBefore, Valid: true},
	})
}

func (s *CleanupStore) PruneEmailVerificationTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	return s.queries.PruneEmailVerificationTokens(ctx, db.PruneEmailVerificationTokensParams{
		ExpiredBefore:  expiredBefore,
		ConsumedBefore: sql.NullTime{Time: consumedBefore, Valid: true},
	})
}

func (s *CleanupStore) PruneEmailChangeTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	return s.queries.PruneEmailChangeTokens(ctx, db.PruneEmailChangeTokensParams{
		ExpiredBefore:  expiredBefore,
		ConsumedBefore: sql.NullTime{Time: consumedBefore, Valid: true},
	})
}

func (s *CleanupStore) PruneSentEmailOutboxRows(ctx context.Context, sentBefore time.Time) (int64, error) {
	return s.queries.PruneSentEmailOutboxRows(ctx, sql.NullTime{Time: sentBefore, Valid: true})
}

func (s *CleanupStore) PruneFailedEmailOutboxRows(ctx context.Context, failedBefore time.Time) (int64, error) {
	return s.queries.PruneFailedEmailOutboxRows(ctx, failedBefore)
}
