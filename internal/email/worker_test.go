package email

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWorkerProcessPendingMarksSent(t *testing.T) {
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
	worker := NewWorker(store, sender, WorkerOptions{
		BatchSize:  5,
		RetryDelay: time.Minute,
		ClaimTTL:   2 * time.Minute,
	})

	if err := worker.ProcessPending(context.Background()); err != nil {
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

func TestWorkerProcessPendingMarksFailedWhenSendFails(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New("provider unavailable")}
	worker := NewWorker(store, sender, WorkerOptions{RetryDelay: 2 * time.Minute})

	if err := worker.ProcessPending(context.Background()); err != nil {
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

func TestWorkerProcessPendingMarksFailedPermanentlyAtMaxAttempts(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Attempts: DefaultMaxAttempts - 1, Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New("bad recipient")}
	worker := NewWorker(store, sender, WorkerOptions{})

	if err := worker.ProcessPending(context.Background()); err != nil {
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

func TestWorkerProcessPendingReturnsClaimError(t *testing.T) {
	worker := NewWorker(&fakeOutboxStore{claimErr: errors.New("database unavailable")}, &fakeSender{}, WorkerOptions{})

	err := worker.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "claim pending email") {
		t.Fatalf("ProcessPending() error = %v, want claim context", err)
	}
}

func TestWorkerProcessPendingReturnsMarkSentError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed:     []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
		markSentErr: errors.New("write failed"),
	}
	worker := NewWorker(store, &fakeSender{}, WorkerOptions{})

	err := worker.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
}

func TestWorkerProcessPendingReturnsMarkFailedError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed:       []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
		markFailedErr: errors.New("write failed"),
	}
	worker := NewWorker(store, &fakeSender{err: errors.New("provider unavailable")}, WorkerOptions{})

	err := worker.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
}

func TestWorkerProcessPendingReturnsMarkFailedPermanentlyError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed:                []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Attempts: DefaultMaxAttempts - 1, Message: Message{To: "user@example.com"}}},
		markPermanentFailedErr: errors.New("write failed"),
	}
	worker := NewWorker(store, &fakeSender{err: errors.New("bad recipient")}, WorkerOptions{})

	err := worker.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
	}
}

func TestWorkerProcessPendingContinuesWhenClaimOwnershipIsLost(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
	}
	store.markSentErr = sql.ErrNoRows
	worker := NewWorker(store, &fakeSender{}, WorkerOptions{})

	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v, want nil", err)
	}
}

func TestWorkerProcessPendingTruncatesLongLastError(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, ClaimToken: "claim-1", Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New(strings.Repeat("x", MaxLastErrorLength+20))}
	worker := NewWorker(store, sender, WorkerOptions{})

	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}
	if len(store.lastError) != MaxLastErrorLength {
		t.Fatalf("last error length = %d, want %d", len(store.lastError), MaxLastErrorLength)
	}
}

func TestWorkerRunRequiresDependencies(t *testing.T) {
	if err := NewWorker(nil, &fakeSender{}, WorkerOptions{}).Run(context.Background()); err == nil {
		t.Fatal("Run() with nil store error = nil, want error")
	}
	if err := NewWorker(&fakeOutboxStore{}, nil, WorkerOptions{}).Run(context.Background()); err == nil {
		t.Fatal("Run() with nil sender error = nil, want error")
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
