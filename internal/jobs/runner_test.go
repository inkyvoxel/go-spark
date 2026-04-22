package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerRunsMultipleJobsIndependently(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var fastRuns atomic.Int32
	slowStarted := make(chan struct{}, 1)
	releaseSlow := make(chan struct{})

	runner, err := NewRunner(logger,
		Job{
			Name:       "fast",
			Interval:   10 * time.Millisecond,
			RunAtStart: true,
			Run: func(ctx context.Context) error {
				fastRuns.Add(1)
				return nil
			},
		},
		Job{
			Name:       "slow",
			Interval:   time.Hour,
			RunAtStart: true,
			Run: func(ctx context.Context) error {
				select {
				case slowStarted <- struct{}{}:
				default:
				}
				<-releaseSlow
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow job did not start")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fastRuns.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if fastRuns.Load() < 2 {
		t.Fatalf("fast job runs = %d, want at least 2", fastRuns.Load())
	}

	cancel()
	close(releaseSlow)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not stop")
	}
}

func TestRunnerRunsJobImmediatelyWhenConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	started := make(chan struct{}, 1)
	runner, err := NewRunner(logger, Job{
		Name:       "immediate",
		Interval:   time.Hour,
		RunAtStart: true,
		Run: func(ctx context.Context) error {
			select {
			case started <- struct{}{}:
			default:
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job did not run at start")
	}
}

func TestRunnerContinuesAfterJobError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var runs atomic.Int32
	runner, err := NewRunner(logger, Job{
		Name:       "flaky",
		Interval:   10 * time.Millisecond,
		RunAtStart: true,
		Run: func(ctx context.Context) error {
			runs.Add(1)
			if runs.Load() == 1 {
				return errors.New("boom")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runs.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	if runs.Load() < 2 {
		t.Fatalf("runs = %d, want at least 2", runs.Load())
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runner did not stop")
	}
}

func TestRunnerRejectsInvalidJobs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := NewRunner(logger, Job{Interval: time.Second, Run: func(ctx context.Context) error { return nil }}); err == nil {
		t.Fatal("NewRunner() with empty name error = nil, want error")
	}
	if _, err := NewRunner(logger, Job{Name: "bad", Run: func(ctx context.Context) error { return nil }}); err == nil {
		t.Fatal("NewRunner() with missing interval error = nil, want error")
	}
	if _, err := NewRunner(logger, Job{Name: "bad", Interval: time.Second}); err == nil {
		t.Fatal("NewRunner() with missing run func error = nil, want error")
	}
}

func TestRunnerStopsCleanlyOnContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var finished sync.WaitGroup
	finished.Add(1)
	started := make(chan struct{}, 1)
	runner, err := NewRunner(logger, Job{
		Name:       "waiter",
		Interval:   time.Hour,
		RunAtStart: true,
		Run: func(ctx context.Context) error {
			select {
			case started <- struct{}{}:
			default:
			}
			defer finished.Done()
			<-ctx.Done()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	cancel()
	finished.Wait()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not stop")
	}
}
