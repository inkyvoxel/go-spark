package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestCleanupJobRunLogsSummary(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	store := &fakeCleanupStore{
		expiredSessions:        2,
		passwordResetTokens:    3,
		emailVerificationToken: 4,
		emailChangeTokens:      5,
		sentOutboxRows:         6,
		failedOutboxRows:       7,
	}
	job, err := NewCleanupJob(store, CleanupOptions{
		Logger:               logger,
		TokenRetention:       24 * time.Hour,
		SentEmailRetention:   7 * 24 * time.Hour,
		FailedEmailRetention: 14 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewCleanupJob() error = %v", err)
	}

	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	entry, ok := findJobLogEntry(t, output.String(), "database cleanup completed")
	if !ok {
		t.Fatalf("missing cleanup summary log in %q", output.String())
	}
	if got := jobAsString(entry["job"]); got != "database-cleanup" {
		t.Fatalf("job = %q, want %q", got, "database-cleanup")
	}
	if got := jobAsInt(entry["deleted_total"]); got != 27 {
		t.Fatalf("deleted_total = %d, want %d", got, 27)
	}
	if got := jobAsInt(entry["duration_ms"]); got < 0 {
		t.Fatalf("duration_ms = %d, want >= 0", got)
	}
}

type fakeCleanupStore struct {
	expiredSessions        int64
	passwordResetTokens    int64
	emailVerificationToken int64
	emailChangeTokens      int64
	sentOutboxRows         int64
	failedOutboxRows       int64
}

func (f *fakeCleanupStore) DeleteExpiredSessions(ctx context.Context, expiredBefore time.Time) (int64, error) {
	return f.expiredSessions, nil
}

func (f *fakeCleanupStore) PrunePasswordResetTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	return f.passwordResetTokens, nil
}

func (f *fakeCleanupStore) PruneEmailVerificationTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	return f.emailVerificationToken, nil
}

func (f *fakeCleanupStore) PruneEmailChangeTokens(ctx context.Context, expiredBefore, consumedBefore time.Time) (int64, error) {
	return f.emailChangeTokens, nil
}

func (f *fakeCleanupStore) PruneSentEmailOutboxRows(ctx context.Context, sentBefore time.Time) (int64, error) {
	return f.sentOutboxRows, nil
}

func (f *fakeCleanupStore) PruneFailedEmailOutboxRows(ctx context.Context, failedBefore time.Time) (int64, error) {
	return f.failedOutboxRows, nil
}

func findJobLogEntry(t *testing.T, output, msg string) (map[string]any, bool) {
	t.Helper()

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := map[string]any{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("json.Unmarshal(%q) error = %v", line, err)
		}
		if got, _ := entry["msg"].(string); got == msg {
			return entry, true
		}
	}
	return nil, false
}

func jobAsString(v any) string {
	s, _ := v.(string)
	return s
}

func jobAsInt(v any) int {
	switch value := v.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}
