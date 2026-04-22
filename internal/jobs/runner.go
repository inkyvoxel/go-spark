package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Name       string
	Interval   time.Duration
	RunAtStart bool
	Run        func(context.Context) error
}

type Runner struct {
	logger *slog.Logger
	jobs   []Job
}

func NewRunner(logger *slog.Logger, jobs ...Job) (*Runner, error) {
	if logger == nil {
		logger = slog.Default()
	}

	for _, job := range jobs {
		if err := validateJob(job); err != nil {
			return nil, err
		}
	}

	return &Runner{
		logger: logger,
		jobs:   append([]Job(nil), jobs...),
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, job := range r.jobs {
		wg.Add(1)
		go func(job Job) {
			defer wg.Done()
			r.runJobLoop(ctx, job)
		}(job)
	}

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (r *Runner) runJobLoop(ctx context.Context, job Job) {
	if job.RunAtStart {
		if !r.runJobOnce(ctx, job) {
			return
		}
	}

	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !r.runJobOnce(ctx, job) {
				return
			}
		}
	}
}

func (r *Runner) runJobOnce(ctx context.Context, job Job) bool {
	startedAt := time.Now().UTC()
	r.logger.Info("background job starting", "job", job.Name, "interval", job.Interval)

	err := job.Run(ctx)
	duration := time.Since(startedAt)
	if err != nil {
		if ctx.Err() != nil {
			r.logger.Info("background job stopped", "job", job.Name, "duration", duration)
			return false
		}
		r.logger.Error("background job failed", "job", job.Name, "duration", duration, "err", err)
		return true
	}

	r.logger.Info("background job finished", "job", job.Name, "duration", duration)
	return true
}

func validateJob(job Job) error {
	if job.Name == "" {
		return fmt.Errorf("background job name is required")
	}
	if job.Interval <= 0 {
		return fmt.Errorf("background job %q interval must be greater than zero", job.Name)
	}
	if job.Run == nil {
		return fmt.Errorf("background job %q run function is required", job.Name)
	}
	return nil
}
