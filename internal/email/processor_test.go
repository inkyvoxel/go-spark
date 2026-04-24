package email

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestProcessorProcessPendingMarksSent(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{
			{
				ID:         1,
				ClaimToken: "claim-1",
				Message: Message{
					From:     "sender@example.com",
					To:       "user@example.com",
					Subject:  "Subject",
					TextBody: "Text",
				},
			},
		},
	}
	sender := &fakeSender{}
	processor := NewProcessor(store, sender, ProcessorOptions{
		BatchSize:  5,
		RetryDelay: time.Minute,
		ClaimTTL:   2 * time.Minute,
	})

	if err := processor.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}

	if store.claimLimit != 5 {
		t.Fatalf("claim limit = %d, want 5", store.claimLimit)
	}
	if store.claimTTL != 2*time.Minute {
		t.Fatalf("claim ttl = %s, want %s", store.claimTTL, 2*time.Minute)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent length = %d, want 1", len(sender.sent))
	}
	if len(store.sentIDs) != 1 || store.sentIDs[0] != 1 {
		t.Fatalf("sent IDs = %v, want [1]", store.sentIDs)
	}
	if len(store.sentClaimTokens) != 1 || store.sentClaimTokens[0] != "claim-1" {
		t.Fatalf("sent claim tokens = %v, want [claim-1]", store.sentClaimTokens)
	}
	if len(store.failedIDs) != 0 {
		t.Fatalf("failed IDs = %v, want none", store.failedIDs)
	}
}

func TestProcessorProcessPendingMarksFailedWhenSendFails(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New("provider unavailable")}
	processor := NewProcessor(store, sender, ProcessorOptions{RetryDelay: 2 * time.Minute})

	if err := processor.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}

	if len(store.failedIDs) != 1 || store.failedIDs[0] != 1 {
		t.Fatalf("failed IDs = %v, want [1]", store.failedIDs)
	}
	if len(store.failedClaimTokens) != 1 || store.failedClaimTokens[0] != "claim-1" {
		t.Fatalf("failed claim tokens = %v, want [claim-1]", store.failedClaimTokens)
	}
	if store.lastError != "provider unavailable" {
		t.Fatalf("last error = %q, want provider unavailable", store.lastError)
	}
	if time.Until(store.retryAt) <= 0 {
		t.Fatalf("retryAt = %s, want future time", store.retryAt)
	}
	if len(store.sentIDs) != 0 {
		t.Fatalf("sent IDs = %v, want none", store.sentIDs)
	}
}

func TestProcessorProcessPendingMarksFailedPermanentlyAtMaxAttempts(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Attempts: DefaultMaxAttempts - 1, Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New("bad recipient")}
	processor := NewProcessor(store, sender, ProcessorOptions{})

	if err := processor.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}

	if len(store.permanentFailedIDs) != 1 || store.permanentFailedIDs[0] != 1 {
		t.Fatalf("permanent failed IDs = %v, want [1]", store.permanentFailedIDs)
	}
	if len(store.permanentFailedClaimTokens) != 1 || store.permanentFailedClaimTokens[0] != "claim-1" {
		t.Fatalf("permanent failed claim tokens = %v, want [claim-1]", store.permanentFailedClaimTokens)
	}
	if len(store.failedIDs) != 0 {
		t.Fatalf("retryable failed IDs = %v, want none", store.failedIDs)
	}
}

func TestProcessorProcessPendingReturnsClaimError(t *testing.T) {
	processor := NewProcessor(&fakeOutboxStore{claimErr: errors.New("database unavailable")}, &fakeSender{}, ProcessorOptions{})

	err := processor.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "claim pending email") {
		t.Fatalf("ProcessPending() error = %v, want claim context", err)
	}
}

func TestProcessorProcessPendingReturnsMarkSentError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed:     []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
		markSentErr: errors.New("write failed"),
	}
	processor := NewProcessor(store, &fakeSender{}, ProcessorOptions{})

	err := processor.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
}

func TestProcessorProcessPendingReturnsMarkFailedError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed:       []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
		markFailedErr: errors.New("write failed"),
	}
	processor := NewProcessor(store, &fakeSender{err: errors.New("provider unavailable")}, ProcessorOptions{})

	err := processor.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
}

func TestProcessorProcessPendingReturnsMarkFailedPermanentlyError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed:                []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Attempts: DefaultMaxAttempts - 1, Message: Message{To: "user@example.com"}}},
		markPermanentFailedErr: errors.New("write failed"),
	}
	processor := NewProcessor(store, &fakeSender{err: errors.New("bad recipient")}, ProcessorOptions{})

	err := processor.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
}

func TestProcessorProcessPendingContinuesWhenClaimOwnershipIsLost(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
	}
	store.markSentErr = sql.ErrNoRows
	processor := NewProcessor(store, &fakeSender{}, ProcessorOptions{})

	if err := processor.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v, want nil", err)
	}
}

func TestProcessorProcessPendingTruncatesLongLastError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New(strings.Repeat("x", MaxLastErrorLength+20))}
	processor := NewProcessor(store, sender, ProcessorOptions{})

	if err := processor.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}
	if len(store.lastError) != MaxLastErrorLength {
		t.Fatalf("last error length = %d, want %d", len(store.lastError), MaxLastErrorLength)
	}
}

func TestProcessorValidateRequiresDependencies(t *testing.T) {
	if err := NewProcessor(nil, &fakeSender{}, ProcessorOptions{}).Validate(); err == nil {
		t.Fatal("Validate() with nil store error = nil, want error")
	}
	if err := NewProcessor(&fakeOutboxStore{}, nil, ProcessorOptions{}).Validate(); err == nil {
		t.Fatal("Validate() with nil sender error = nil, want error")
	}
}

func TestProcessorProcessPendingLogsCycleSummary(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{
			{ID: 1, ClaimToken: "claim-1", Message: Message{To: "sent@example.com"}},
			{ID: 2, ClaimToken: "claim-2", Message: Message{To: "retry@example.com"}},
			{ID: 3, ClaimToken: "claim-3", Attempts: DefaultMaxAttempts - 1, Message: Message{To: "perm@example.com"}},
		},
	}
	sender := fakeSenderFunc(func(ctx context.Context, message Message) error {
		if strings.HasPrefix(message.To, "sent@") {
			return nil
		}
		return errors.New("smtp failed")
	})
	processor := NewProcessor(store, sender, ProcessorOptions{
		Logger: logger,
	})

	if err := processor.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}

	entry, ok := findLogEntry(t, output.String(), "email outbox cycle completed")
	if !ok {
		t.Fatalf("missing cycle summary log in %q", output.String())
	}
	if got := asInt(entry["claimed"]); got != 3 {
		t.Fatalf("claimed = %d, want %d", got, 3)
	}
	if got := asInt(entry["sent"]); got != 1 {
		t.Fatalf("sent = %d, want %d", got, 1)
	}
	if got := asInt(entry["retry_scheduled"]); got != 1 {
		t.Fatalf("retry_scheduled = %d, want %d", got, 1)
	}
	if got := asInt(entry["permanent_failures"]); got != 1 {
		t.Fatalf("permanent_failures = %d, want %d", got, 1)
	}
	if got := asInt(entry["skipped_claim_lost"]); got != 0 {
		t.Fatalf("skipped_claim_lost = %d, want %d", got, 0)
	}
	if got := asInt(entry["duration_ms"]); got < 0 {
		t.Fatalf("duration_ms = %d, want >= 0", got)
	}
}

func TestProcessorProcessPendingSkipsSummaryWhenNoWork(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	processor := NewProcessor(&fakeOutboxStore{}, &fakeSender{}, ProcessorOptions{
		Logger: logger,
	})

	if err := processor.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}
	if strings.Contains(output.String(), "email outbox cycle completed") {
		t.Fatalf("unexpected cycle summary log: %q", output.String())
	}
}

type fakeOutboxStore struct {
	claimed                    []OutboxEmail
	claimLimit                 int64
	claimTTL                   time.Duration
	claimErr                   error
	markSentErr                error
	markFailedErr              error
	markPermanentFailedErr     error
	sentIDs                    []int64
	sentClaimTokens            []string
	failedIDs                  []int64
	failedClaimTokens          []string
	permanentFailedIDs         []int64
	permanentFailedClaimTokens []string
	lastError                  string
	retryAt                    time.Time
}

func (s *fakeOutboxStore) ClaimPending(ctx context.Context, now time.Time, claimTTL time.Duration, limit int64) ([]OutboxEmail, error) {
	s.claimLimit = limit
	s.claimTTL = claimTTL
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return s.claimed, nil
}

func (s *fakeOutboxStore) MarkSent(ctx context.Context, id int64, claimToken string, sentAt time.Time) error {
	if s.markSentErr != nil {
		return s.markSentErr
	}
	s.sentIDs = append(s.sentIDs, id)
	s.sentClaimTokens = append(s.sentClaimTokens, claimToken)
	return nil
}

func (s *fakeOutboxStore) MarkFailed(ctx context.Context, id int64, claimToken, lastError string, availableAt time.Time) error {
	if s.markFailedErr != nil {
		return s.markFailedErr
	}
	s.failedIDs = append(s.failedIDs, id)
	s.failedClaimTokens = append(s.failedClaimTokens, claimToken)
	s.lastError = lastError
	s.retryAt = availableAt
	return nil
}

func (s *fakeOutboxStore) MarkFailedPermanently(ctx context.Context, id int64, claimToken, lastError string, failedAt time.Time) error {
	if s.markPermanentFailedErr != nil {
		return s.markPermanentFailedErr
	}
	s.permanentFailedIDs = append(s.permanentFailedIDs, id)
	s.permanentFailedClaimTokens = append(s.permanentFailedClaimTokens, claimToken)
	s.lastError = lastError
	return nil
}

type fakeSender struct {
	err  error
	sent []Message
}

func (s *fakeSender) Send(ctx context.Context, message Message) error {
	if s.err != nil {
		return s.err
	}
	s.sent = append(s.sent, message)
	return nil
}

type fakeSenderFunc func(ctx context.Context, message Message) error

func (f fakeSenderFunc) Send(ctx context.Context, message Message) error {
	return f(ctx, message)
}

func findLogEntry(t *testing.T, output string, msg string) (map[string]any, bool) {
	t.Helper()

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := map[string]any{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("json.Unmarshal(%q) error = %v", line, err)
		}
		if value, _ := entry["msg"].(string); value == msg {
			return entry, true
		}
	}
	return nil, false
}

func asInt(v any) int {
	switch value := v.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}
