package email

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWorkerProcessPendingMarksSent(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{
			{
				ID: 1,
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
	})

	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}

	if store.claimLimit != 5 {
		t.Fatalf("claim limit = %d, want 5", store.claimLimit)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent length = %d, want 1", len(sender.sent))
	}
	if len(store.sentIDs) != 1 || store.sentIDs[0] != 1 {
		t.Fatalf("sent IDs = %v, want [1]", store.sentIDs)
	}
	if len(store.failedIDs) != 0 {
		t.Fatalf("failed IDs = %v, want none", store.failedIDs)
	}
}

func TestWorkerProcessPendingMarksFailedWhenSendFails(t *testing.T) {
	store := &fakeOutboxStore{
		claimed: []OutboxEmail{{ID: 1, Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New("provider unavailable")}
	worker := NewWorker(store, sender, WorkerOptions{RetryDelay: 2 * time.Minute})

	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}

	if len(store.failedIDs) != 1 || store.failedIDs[0] != 1 {
		t.Fatalf("failed IDs = %v, want [1]", store.failedIDs)
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
		claimed: []OutboxEmail{{ID: 1, Attempts: DefaultMaxAttempts - 1, Message: Message{To: "user@example.com"}}},
	}
	sender := &fakeSender{err: errors.New("bad recipient")}
	worker := NewWorker(store, sender, WorkerOptions{})

	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatalf("ProcessPending() error = %v", err)
	}

	if len(store.permanentFailedIDs) != 1 || store.permanentFailedIDs[0] != 1 {
		t.Fatalf("permanent failed IDs = %v, want [1]", store.permanentFailedIDs)
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
		claimed:     []OutboxEmail{{ID: 1, Message: Message{To: "user@example.com"}}},
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
		claimed:       []OutboxEmail{{ID: 1, Message: Message{To: "user@example.com"}}},
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
		claimed:                []OutboxEmail{{ID: 1, Attempts: DefaultMaxAttempts - 1, Message: Message{To: "user@example.com"}}},
		markPermanentFailedErr: errors.New("write failed"),
	}
	worker := NewWorker(store, &fakeSender{err: errors.New("bad recipient")}, WorkerOptions{})

	err := worker.ProcessPending(context.Background())
	if err == nil {
		t.Fatal("ProcessPending() error = nil, want error")
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
	claimed                []OutboxEmail
	claimLimit             int64
	claimErr               error
	markSentErr            error
	markFailedErr          error
	markPermanentFailedErr error
	sentIDs                []int64
	failedIDs              []int64
	permanentFailedIDs     []int64
	lastError              string
	retryAt                time.Time
}

func (s *fakeOutboxStore) ClaimPending(ctx context.Context, now time.Time, limit int64) ([]OutboxEmail, error) {
	s.claimLimit = limit
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return s.claimed, nil
}

func (s *fakeOutboxStore) MarkSent(ctx context.Context, id int64, sentAt time.Time) error {
	if s.markSentErr != nil {
		return s.markSentErr
	}
	s.sentIDs = append(s.sentIDs, id)
	return nil
}

func (s *fakeOutboxStore) MarkFailed(ctx context.Context, id int64, lastError string, availableAt time.Time) error {
	if s.markFailedErr != nil {
		return s.markFailedErr
	}
	s.failedIDs = append(s.failedIDs, id)
	s.lastError = lastError
	s.retryAt = availableAt
	return nil
}

func (s *fakeOutboxStore) MarkFailedPermanently(ctx context.Context, id int64, lastError string, failedAt time.Time) error {
	if s.markPermanentFailedErr != nil {
		return s.markPermanentFailedErr
	}
	s.permanentFailedIDs = append(s.permanentFailedIDs, id)
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
